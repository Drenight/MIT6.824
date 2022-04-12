# 2. Design Overview
## 2.4 Single Master

### interactions for a simple read
1. client sends master (fileName, chunk index), caculated by fixed chunk size and byte offset
2. master reply in (chunk handle, chunk locations of replicas), client caches.
3. client sends one of replicas(closest) in (chunk handle, byte range)

## 2.5 Chunk Size
> However, hot spots did develop when GFS was first used

Q:seems big chunk size brings hotspot problem?

A:
[stackOverflow related question](https://stackoverflow.com/questions/46577706/why-do-small-files-create-hot-spots-in-the-google-file-system)
In simple terms, if the chunk size is lower, the chance of **simultaneous** access to the same chunk is lower, under the scenario of 1000 clients & 10 servers. All clients will become not completely synchronized due to nature of network, clients' working load, etc; Another explaination is that the paper's point is to say in practical application, not all clients end up reading the entire file at all. It's not about chunk size's business.

## 2.6 Metadata

### three major (stored in memory so pretty quick)
  1. file and chunk namespaces
  2. mapping from files to chunks
  3. locations of each chunk's replicas

- capacity whole system limited by master's memory(metadata is in memory), prefix compression, 64bytes expressing 64MB chunk
- operation log, lose it lose all, replicate remote, respond client's operation after flush to disk re&lo
-  checkpoint, B-tree

## 2.7 Consistency Model (GFS's guarantees)
- File namespace(e.g., file creation) are atomic
- write, record appends(atomically at least once)
- successful mutations -> file region is **guaranteed** to be defined and contain the last mutation, by
  1. apply mutas all replicas in same order
  2. chunk version prevent stale record
- cache purge
- checksum for bad chunkservers

### APP's accommodate
- practically mutate files by appending > overwriting (more efficient and resilient for failures), checkpoints allow restart incrementally

# 3. System Interactions

## 3.1 Leases and Mutation Order
- each mutation -> all chunk's replicas
- the chunk got lease -> the primary, who can pick a serial order for all mutations, other replicas follow this order, request piggybacked in HeartBeat to extend
- revoke by master, e.g. disable mutations on a file being renamed

### interactions for a simple write
1. client asks master which chunkserver holds the currenct lease for the chunk and the locations of other replicas
2. master replies (identity of primary, locations of secondary replicas), client caches it
3. client pushes data to all relicas(any order), chunkserver store the data in an LRU buffer until the data is used or aged out(LAZY).
4. after all replicas ack, client sends write req to primary, primary assigns consecutive serial numbers to mutations received from multiple clients, apply mutation to its own local state
5. primary forwards write req to all secondary
6. all secondaries reply to primary, they done
7. primary reply client, any error in any replicas reported, retry from 3->7

- large write or straddles chunbk boundary, breaks down into multi operations, may be interleaved by concurrent, consistent but undefined

## 3.2 Data Flow
  > We decouple the flow of data from the flow of control to use the network efficiently
  - control flow: client->primary->all secondaries
  - data flow: pushed linearly along a chain of chunkservers in a pipelined fashion

## 3.3 Atomic Record Appends
- client specifies only the data, GFS appends it to the file at least once atomically
- like in 3.1, primary append its replica, tell secondaries write data, exact offset of the primary
- any replica error, client retry
- dont guarantee replicas bytewise identical, guarantee written at least once as an atomic unit
- all replicas at least as long as the end of record, future record at higher offset no matter who's primary

## 3.4 Snapshot
- To create branch copies of huge data sets / checkpoint the current state
- **copy-on-write**

### procedure
1. master revoke leases on the chunks in the files about to snapshot, force contact master
2. master log ops to disk, apply to its in-memory state by duplicating the metadata for directory tree, point to the same chunks as the source files
3. when client req write chunk C, master notice reference count for chunk C is greater than one, pick new chunk handle C', ask each chunserver has C to create new chunk C'

Q: The paper mentions reference counts -- what are they?

A: They are part of the implementation of copy-on-write for snapshots.
When GFS creates a snapshot, it doesn't copy the chunks, but instead
increases the reference counter of each chunk. This makes creating a
snapshot inexpensive. If a client writes a chunk and the master
notices the reference count is greater than one, the master first
makes a copy so that the client can update the copy (instead of the
chunk that is part of the snapshot). You can view this as delaying the
copy until it is absolutely necessary. The hope is that not all chunks
will be modified and one can avoid making some copies.

- just like lazy in segment tree

# 4. Master Operation

## 4.1 Namespace Management and Locking
- master operations take long time, use lock not to delay other operations
- GFS = full pathnames -> metadata
- master op acquires a set of locks, op involve /d1/d2/.../dn/leaf
  - read-locks on /d1, /d1/d2, ..., .../dn
  - either a read/write-lock on /d1/d2/.../dn/leaf
- prevent a file /home/user/foo from being created while /home/user is being snapshotted to /save/user
- this locking scheme allows concurrent mutations in same direction

## 4.3 Creation, Re-repication, Rebalancing

### Creation
chunk creating principle
1. place new replicas on chunkservers with below-average disk space utilization
2. limit number of "recent" creations on each chunkserver
   - predicts imminent heavy write traffic
3. spread replicas of chunk across racks

### Re-replication
when number of available replicas falls below user's goal

## 4.4 Garbage Collection
lazily recalim until regular garbage collection at both file & chunk levels

---

# note from lec3
- split brain: >1 primarys, processing reqs without knowing each other
  - usually caused by net partition, master can't talk to primary while primary can talk to clients, so the master must wait until lease expire to designate a new primary
- version number only inc when master assign a new primary

