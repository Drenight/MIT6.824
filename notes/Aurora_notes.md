# Q&A
Aurora's down-side?
- lose generality, Aurora storage system is not useful for anything other than Aurora
  - Aurora storage servers know a lot about database
  - significant chunk of database functionality $\rightarrow$ storage system

# Abstract 

- quick skim about Aurora

What's Aurora?
- a relational database service
- for OLTP(Online Transaction Processing) workloads

Assumption about "What's the bottle-neck in high throughput data processing"
- compute & storage $\rightarrow$ network, since 2017

What Aurora do focusing network optimization?
- A novel architecture
  - push **redo processing** to a multi-tenant scale-out(水平扩展) storage service(who is purpose-built for Aurora)
    - not only reduce network traffic
    - but also allows for fast crash recovery

# 1 Introduction

Cloud?
- Many IT workloads are moving to public cloud providers

Why decouple compute from storage & replicate storage across multiple nodes?
- Handle replacing meisbehaving or unreachable hosts
- Add replicas
- Fail over from writer to a replica
- Scaling adjust

I/O change?
- can be spread across many nodes&disks
- individual disk s are no longer hot

Operations can't avoid synchronous & stall?
- A disk read due to a miss in database buffer cache
  - the reading thread can't progress until read completes
- Transaction commits
  - will inhibit others from progressing, s.t.2-phase commit

# 2 Durability at Scale

## 2.1 Replication and Correlated Failures
- Instance lifetime not correlate well with storage lifetime $\rightarrow$ better decouple them
  - customers shut them down
  - They resize them up and down based on load
- Once we decouple them, storages nodes can also fail
  - So they must be replicated in some form to provide resiliency to failure
    - a transient lack of network availability
    - temporary downtime on a reboot
  - One way to tolerate failures in a replicated system: Use a quorum-based voting protocol

Why $V_w + V_r > V$?
  - to let read and write have intersection

What's Availability Zone(AZ)?
- a subset of a region(imagine region as a dataCenter)
- connected to other AZ with low latency links
- isolated from other AZ by some ways, e.g. physical different power supply?
- Therefore failure **HARDLY AFFECT ACROSS AZ**

Aurora's quorum? Why?
- Whole AZ fail may cause huge quorum reduction
  - IGUESS that's why we turn down $V_r$ to 3 other than 4
- replicate each data, across 3 AZs, with 2 copies in each AZ
- $V=6$, $V_w=4$, $V_r=3$
- Tolerate 
  - losing AZ+1(entire AZ and one additional node) without losing data
    - can still support read
  - losing entire AZ without impacting write
  - Ensuring read quorum enables us to rebuild write quorum by adding extra replica copies

## 2.2 Segmented Storage

What's sgemented storage?
- Partition the databse volume into small fixed size segments(10GB now)
  - each replicated 6 ways into Protection Groups, each PG consistes of 6 10GB segments
    - organized across 3 AZs, with 2 segments in each AZ 
- A storage volume is a concatenated set of PGs
  - physically implemented using a large fleet of storage nodes, provisioned as virtual hosts with attached SSDs using Amazon EC2

segment $\subset$ DB volume $\subset$ whole DB

Segment?
- unit of independent background noise failure and repair
  - A 10GB segment can be repaired in 10 seconds on a 10Gbps network link

Hardness to break such durability?
- Requirements?
  - need to see two such failures in the same 10 second window
  - plus a failure to lose quorum
- Sufficiently unlikely

## 2.3 Operational Advantages of Resilience
- Due to analysis above, our system has a high tolerance to failures
  
How can we leverage the high tolerance?
- safe to do maintenance operations that can cause segment unavailability
  - e.g. **heat management**
    - mark one of the segments on a hot disk as bad
    - the quorum will be quickly repaired by migration to some other colder node in the fleet
  - e.g. **software upgrades**
    - execute them one AZ at a time, ensure no more than one member of a PG is being patched simultaneously
- Tolerate us for agile deployments in storage service

What will quorum do?
- read from the survived at least 3 segments, add extra node to recover quorum
- then that segment(10 GB portion of whole DataBase) will become write available again 

# 3 The log is the Database
- Combine above "segmented replicating" with traditional database is not a tenable efficient solution for cloud scenario

How Aurora improves?
- offload log processing to the storage sevice
- experimentally demonstrate how it can **DRAMATICALLY REDUCE NETWORK IOs**
- Some sechniques to minimize synchronous stalls and unnecessary writes

## 3.1 The Burden of Amplified Writes

each segment 6 ways
- pros: high resilience
- cons: untenable performance for traditional DB who generates many real I/Os for each write
  - they are amplified by 6x replication
  - heavey packets per second(PPS) burden

How traditional DB works on WRITE?
- write data pages to objects it exposes(b-tree/heap file..)
- write redo log to a write-ahead log (WAL)
  - >预写日志(WAL,Write Ahead Log)是关系型数据库中用于实现事务性和持久性的一系列技术。 简单来说就是，做一个操作之前先讲这件事情记录下来。
  - **Write to log, before really write the data**
  - redo log is for storing dirty page(has been modified)

a synchronous HA traditional MySQL across dataCenter network working in active-standby, mirror what?
- this is also Amazon's RDS(Relational Database Service)'s architecture
1. redo log
2. binary statement log(archived to Amazon Simple Storage Service(S3))
   1. to support point-in-time restores(时间点恢复, recover with granularity of second!)
   2. [S3 vs EBS](https://www.geeksforgeeks.org/difference-between-aws-s3-and-aws-ebs/)
      1. S3: object-level storage
      2. EBS: block-level storage
3. modified data pages(real changed data)
   1. TODO, why still mirror this? We have redo log.
      1. Awesome! This paper is doing this, only mirror redo log.
         1. Remember this paper's "push down redo log"?
         2. The reason for the replication here is that the logic is separated, the storage layer does the work of the storage layer, and the transaction processing layer does the work of the transaction processing layer
         3. In this original architecture, write instructions are processed on the primary side, and as many transaction instructions as are generated are mirrored.
4. second temporary write of the data page(double-write)
   1. to prevent torn pages
   2. >The InnoDB double write buffer helps recover from half-written pages. Whenever InnoDB flushes a page from the buffer pool, it is first written to the double write buffer. Only if the buffer is safely flushed to the disk, will InnoDB write the pages to the disk. So that when InnoDB detects the corruption from the mismatch of the checksum, it can recover from double write buffer. The PostgreSQL equivalent is full-page writes.
   3. to make sure you have a backup correct page to recover from, during a non-atomic page writing
5. metadata file(FRM)
   1. >These form files are used for defining the fields in the table. It is also used to represent the formatting information about the structure of the table.

Acutal IO flow order?
- step 1: writes are issued to EBS
- step 2: write local AZ's EBS mirror, ACK received when both local EBS are done
- step 3: write is staged to the standby instance usinig **synchronous block-level software mirroring**
- step 4: writes are written to the standby EBS volume
- step5: write to standby associated mirror

Mirrored MySQL model's drawbacks?
- First, step 1,3,5 are sequential and synchronous, bring latency
- Second, user operations that are a result of OLTP applications cause many different types of writes often representing the same information in multiple ways
  - e.g. the double write 
    - exactly my concern, why mirror this

## 3.2 Offloading Redo Processing to Storage

Aurora's OPs across network?
- Only redo log
  - log applicator is pushed to the storage tier
    - it generates database pages in **background**
      - generating each page from the beginning is expensive
        - we continually materialize database pages in the background
  - THE LOG IS THE DATABASE

Background Materialization versus Checkpointing
- Only pages with a long chain of modifications need to be rematerialized
- Checkpointing is governed by the entire redo log length
  - Aurora based on length of page's chain

Benefits?
- dramatically reduces network load, despite amplifying writes to replications
- storage service sacle out I/Os
  - without impacting write throughput of the database engine
    - TODO ??? Because it doesn't stall any instance?, like the traditional architecture's step 1,3,5?

How IO flow works for a write?
- IO flow batches fully ordered log records to the same PG
- delivers each batch to all 6 replicas, the batch will be persisted on disk
- database engine wait ACK from 4 out of 6 to satisfy write quorum
- replicas use the redo log to apply changes to their buffer caches

Write only, synchronous mirrored MySQL vs Aurora
- 35x efficiency

## 3.3 Storage Service Design Points

Core design tenet?
- move processing to background, minimize the foreground latency
  - background generate image based on log

Aurora's storage nodes' IO traffic
1. receive log record, add to in-memory queue
2. persist record on disk, wait ACK
3. organize records, identify gaps in the log(Some batches may be lost)
4. gossip with peers to fill in gaps
5. coalesce log records into new data pages
6. periodically stage log and new pages to S3
7. periodically garbage collect old versions
8. periodically validate CRC codes on pages

# 4 The log marches forward

## 4.1 Asynchronous Processing
- In section 3, we model the databasee as a redo log stream
  - we have the fact, log advances as an ordered sequence of changes
  - In practice, each log record has a Log Sequence Number(LSN), monotonically increasing

How LSN affect consensus protocol for maintaing state?
- approach the problem in an asynchronous fashion instead of using a chatty protocol like 2PC
- we maintain points of consistency and durability, continually advance these points when we ack from storage requests
- storage nodes gosspi with each other to find LSN gap
- Since we maintain runtime state, we can read from single node instead of quorum read

How to recover a uniform view of storage for interrupted transactions after rebooting?
- During recovery after the database server crashes and is restarted, Aurora has to make sure that it starts with a complete log.
- Difficuity?
  - There may be holes near end of log, if old DB server wasn't able to get them replicated on storage server.
<!-- logic dealing above is kept in database engine
  - before database is allowed to access storage volue, storage service does its own recovery, focusing on uniform view-->
- Recovery software scans the log(use quorum reads), find the highest LSN public kept, that's VCL
- After find VCL, recovery software tells STORAGE SERVER to DELETE ALL LOG ENTRIES AFTER VCL
  - guarantee not to include log from committed transactions
    - Because no transaction commits,(reply to client) until all log entries **up through the end of the transaction** are in a write-quorum of storage servers 
      - commited logs must be in front of 

How LSN help above?
- storage service determines the highest LSN for which it can guarantee availability for all prior log records
  - call it VCL, Volume Complete LSN
- 


## 4.2 Normal Operation
- write, read, commit, replica

### 4.2.1 Writes

# 5 Putting it all together

# 6 Performance results

# 7 Lessons learned

# 8 Related work

# 9 Conclusion

# 10 Acknowledgments

# 11 Reference