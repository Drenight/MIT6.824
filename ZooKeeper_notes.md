# Lec 8
It's ok for reply of resended request to be as stale as the time point when initial request was sent.(30:00)

Why ZK?
1. API general-purpose coordination service
2. Nx servers$\rightarrow$? Nx performance

Why ZK Problem 2
- ZK model?
  - hundreds of clients
  - ZK is a layer running on top of ZAB
  - ZAB works just like RAFT, maintaing a log, and atomiclly broadcast, majority...

- In real world, a huge number of workloads are read heavy
  - Maybe we can send "write" to leader, and just send "read" to followers!
    - That will make huge progress for Nx servers Nx performance
    - Actually, ZK cluster's read performance increase dramatically as you add more machines 
    - **Key problem: can't believe any replica except leader to be up2date**

- If we want to build a linearizable system, we can not rely on this may not be up2date

- How ZooKeeper deal with this not up2date?
  - ZK not obliged to provide fresh data to read, it's legal to return stale read result
  - A classic solution trade-off High Performance and Strong Consistency, give up "strong"
  - Here comes the question, why you think "permit to read stale" can still produce a useful system?

ZK Guarantees?
1. LINEARIZEABLE WRITES
   - Even clients might submit writes concurrently, system behave as if it excutes writes ONE AT A TIME, in SOME ORDER, obey real-time ordeing
2. FIFO CLIENT ORDER
   - For any given client, its operations execute in his specified order
     - write: clear client-specified order
       - client is allowed to launch **asynchronous** sequence of writes without waiting for any of them to complete
       - the professor guess: the clie nt actually stamps writes with numbers increasing: "this one first, this one second..."
     - read: all read obey **monotonic consistency**
       - every read reflects some exact time point's replica's kv looks like
       - the successive reads have to observe at some point **don't go backwards**
       - After crash client reconnect whole service, connect only to newer followers / first chosen server will keep stuck until it get up2date, paper didn't mention.
         - Using **ZXID** of preceding log entry
   - This FIFO feature implies read-after-write consistency
     - Because the server you read will stall until it commit previous write
     - TODO still wait-free, cause client request is async

✨✨✨Here comes the interesing point: Why guarantee can't prevent stale read?
- These two guarantee is **ONLY FOR SINGLE CLIENT HIMSELF**
- If client $A$ writes, $B$'s read won't be guaranteed read that write 

✨✨✨Sync operation?
- essentially a write operation, makes its way through the system as if it were a write
- finally showing up in the logs of the replica client is talking to
- tell the replica don't serve this read until you have seen my last sync

exists(ready, watch=true)

[kafka replace zk with raft, why?](https://www.confluent.io/blog/why-replace-zookeeper-with-kafka-raft-the-log-of-all-logs/)
- TODO, system rely on system $\rightarrow$ system rely on inner algorithm?

## More about ZK

Motivation
- Test-and-Set VMFT needed
- publish dynamic configuration

How we do increment?
```c
While true{
  x,v = GETDATA("f");
  if(SET("f",x+1,v)){
    break;
  }
}
```
- leader will return false if version != v 

ZK's lock and go thread's lock?
- ZK's lock itself doesn't guarantee atomicity
  - previous holder may crash, in GO there's no notion of threads failing
    - Actually GO's thread may fail, action like divide 0, professor adviced to kill the broken program, cuz lock is not safe any more
      - it will be very hard to play the game like setting a "ready" flag to recover from a crash of a thread
  - every one should be prepared to clean up from some previous disaster write
- Recall my MAPREDUCE implement, master mark the job locked cause it's assigned to a worker, if long time passed master will release that lock and let other worker reexecute the job.
  - Previous worker only write the intermediate temporary file before job done, so it implies "CLEAN UP"
  - This lock is called "SOFT LOCK"

# Abstract
What function?
- A service for coordinating process of distributed applications.
- Provid a simple and high performance KERNEL for building more complex coordination primitives at the client.
- Incorporate GROUP MESSAGEING, SHARED REGISTERS, DISTRIBUTED LOCK
- WAIT-FREE
- LINEARIZABILITY for all requests

# 1 Introduction
Large-scale distributed applications require coordination. Configuration is the most basic requirement.
- DYNAMIC configureation parameters

Design towards?
- Opt for exposing an API that enables application developers to implement their own primitives.
  - led to the implementation of a COORDINATION KERNEL, enables new primitivies without requiring changes to the service core.
    - enables multiple forms of coordination adapted to the requirements of applications

Wait-Free?
- Remove lock, can cause wait & blocking, harm to performance, app become complicated cause need to depend on responses and failure detection of other clients
- Instead, manipulate simple wait-free data objects
  - organized hierarchically as in file systems

Wait-free is not sufficient for coordination, how to fix?
- guarantee the order of OPs
  - guarantee both FIFO client ordering of OPs and linearizable writes
    - enable an efficient implementation of the service; sufficient to implement coordination primitives

ZooKeeper implement architecture?
- a simple PIPELINED architecture
  - naturally enables the execution of OPs from a signle client in FIFO
  - Enable clients to sumbit OPs asynchronously

How update satisfy linearizability?
- use a leader-based atomic broadcast protocol, called Zab

Main contriibutions of this paper:
1. **Coordination kernel**: build a wait-free coordination service
2. **Coordination recpies**: how to build higher level coordination primitives(even blocking and strongly consistent) based on the wait-free kernel
3. **Experience with Coordination**: share some ways how author use ZooKeeper

# 2 The ZooKeeper service
- Client submit requests to ZooKeeper through a client API

Terminology?
- client: user of the ZK service
- server: a PROCESS providing ZK service
- znode: an in-memory data node in the ZK data, organized in a hierarchical namespace, refered to as the data tree
- session: client establish when connect to ZK and obtain a session handle, throught it to issue requests

## 2.1 Service overview
ZK provides its clients the abstraction of a set of data nodes(znodes), organized accoring to a hierarchical name space

Types of znode?
1. Regular: client create&delete
2. Ephemeral: client create, can be del by system automatically when session terminate
   - When system believe client died 
3. SEQ, appended with a number by ZK, which is never repeated

Sequential flag?
- father's is smaller than son's
- monotonically increasing

Watch?
- allow clients to receive timely notifications of changes, without requiring polling
- client issue a read OP with a watch flag set
  - the server will notify the client when the information returned has changed
  - one-time trigger, associated with a session
    - unregister once triggered / session termi
  - **just indidcate change happen, not show change to what**
  - ```getData("/foo",watchflag = true)```, before "/foo" change twice, client will get a notify

Data model?
- A file system with a simplified API, only full data reads and writes, or a key/value table with hierarchical keys

znodes functions like what?
- map to abstractions of the client application

Session and ZK ensemble?
- Sessions enable a client to move transparently from one server to another within a
ZK ensemble, hence persist across ZK servers.

## 2.2 Client API

- ```create(path, data, flags)```: Creates a znode with pathname ```path```, stores ```data```, return the name of the new znode.
  - ```flags``` enable a client to select the type of znode: regular, ephermeral and the sequential flag
  - They are all EXCLUSIVE, return "NO" if exists
- ```delete(path, version)```: Delets the znode```path```, if the znode is at the expected ```version```
- ```exists(path, watch)```
- ```getData(path, watch)```
- ```setData(path, data, version)```
- ```getChildren(path, watch)```: return the set of names of the children of a znode
- ```sync(path)```: Wait for all updates pending at the operation to propagate to the sever that the client is connected to. path ignored currently.

All methods have a synchronous and an asynchronous version

ZooKeeper not maintain handle to access znode, use full file name, simplifier.

**All update methods take an expected version number, for conditional updates**
- version not match, return fail

## 2.3 ZooKeeper guarantees

Two basic ordering guarantees:
1. Linearizable writes
   - all update are serializable, respect precedence
2. FIFO client order
   - same to client's sending order 

Difference between A-linearizability and ZooKeeper's linearizable?
- A-linearizability
  - a client is only able to have one outstanding operation at a time
  - **(client is one thread)**
  - originally proposed by Herlihy
- ZooKeeper's linearizable
  - we allow a client to have multiple outstanding operations, and guarantee their order follow FIFO

what happens if a process sees that ready exists before the new leader starts to make a change and then starts reading the configuration while the change is in progress?
- TODO not understand

What's slow read?
- a **read** leaded by a **sync**

Two other guarantee about liveness?
1. majority live, it is available
2. a successful change request, can persist across any number of failures, as long as a quorum of servers is eventually able to recover

## 2.4 Examples of primitives
- implement stronger primitives based on ZooKeeper
  
Why we can build some many powerful primitives?
- All because ZK's two basic ordering guarantees
  - all update follows FIFO, you will always see preceding write
    - TODO, more details

Stronger primitives?
1. Configuration Management
   - simplest dynamic config in distributed application 
   - config stored in znode $z_c$, obtain config by reading $z_c$ with watch flag set to true.
   - >If the configuration in $z_c$ is ever updated, the processes are notified and read the new configuration, again setting the watch flag to true.
   - watch guarantee a process has the most recent information

2. Randezvous
   - why use? Cause not always clear a priori, what the final system cofig will look like
     - e.g.: Client wants to start master and some worker process, don't know how to config workers' config of master's address
   - randerzvous znode $z_r$, created by client, pass that node to worker init(), master will fill that node and worker will use it by watch

3. Group Membership
   - seems trivial
  
4. Simple Locks

    Simplest locking protocol based on ZK?
    - Although ZK is not a lock service, it can be used to implement locks
    - simplest lock: "lock files"
        - A client tries to create the designated znode with EPHEMERAL flag, 
            - if succeeds, hold that lock
            - if fail, read znode with watch to monitor leader dies
        - leader dies or explicitly deletes the znode, and trigger notify to those watching clients

    Above protocl has any problems?
    1. "herd", What's herd?
        - 惊群
        - Massive clients will be triggered
        - Non-scalable lock
    2. Only implementing EXCLUSIVE locking
        - reads can't not parallel

5. Simple loscks without herd effect
   - Line up all the clients requesting the lock, each client obtains the lock in order of request arrival
   - use SEQUENTAIL to order client's attempt
      ```
      Lock
      1 n = create(l + “/lock-”, EPHEMERAL|SEQUENTIAL)
      2 C = getChildren(l, false)
      3 if n is lowest znode in C, exit
      4 p = znode in C ordered just before n
      5 if exists(p, true) wait for watch event
      6 goto 2
      Unlock
      1 delete(n)
      ```

   - Example graph of lock without herd (I guess like this)
     
    ``` 
      znode l "/p"(SEQ=37)
          |--- znode child1 "/p/lock1022" (SEQ=1022), hold lock
          |--- znode child2 "/p/lock1033" (SEQ=1033), watch child1
          |--- znode child3 "/p/lock1095" (SEQ=1095), watch child2   

      ```

    - client only watch znode precedes his znode, hence only awake one process

6. Read/Write Locks
    ```
    Read Lock
    1 n = create(l + “/read-”, EPHEMERAL|SEQUENTIAL)
    2 C = getChildren(l, false)
    3 if no write znodes lower than n in C, exit
    4 p = write znode in C ordered just before n
    5 if exists(p, true) wait for event
    6 goto 3
    ```
    - Briefly
        - read client watch precedent write lock
        - write client watch precdent every lock   
    - all read client "herd"?
        - we need this effect, they all permit now
7. Double Barrier
    - enable client sync beginning and end of a computation
    - wait enough(over barrier threshold) processed join barrier, processes start, leave the barrier once all processes have finished

# 3 ZooKeeper Applications
# 4 ZooKeeper Implementation

ZK workflow?
1. server receive client's request
2. request processor prepare for execution
3. if request requires coordination among servers(like write), then call agreement protocol service(atomic broadcast)
4. server commit change to ZooKeeper database, fully replicated across all servers of the ensemble
5. In case of read, just read local database

ZK's DB?
- in-memoery
- each znode stores maximum 1MB data by default

## 4.1 Request Processor

Will local replicas diverge?
- Since messaging layer is atomic, local replicas are guaranteed never diverge

Transactions are said to be idempotent, which means redundant message of leader's transactions are safe, how it is implemented?
- When leader receives a write request, it calculates what the state of the system will be(TODO, detail), when the write is applied and transforms it into a transaction that captures this new state.
- Future state must be calculated because there may be outstanding transactions havent yet applied to the DB
- e.g. client does a conditional ```setData```, the version number in request matches the **future** version number of the znode being updated, the service generates a ```setDataTXN``` contains the new data, the new version number, updated time stamps; 
  - Else like mismatch of version number, generate an ```errorTXN```

## 4.2 Atomic Broadcast
Request flow?
- All requests that **update**(TODO ps: Read is in local replica/nearest followers) ZooKeeper state are forwarded to the leader.
- Leader executes the request and broadcast the change to the ZooKeeper state through ```Zab```, an atomic broadcast protocol.
- Server responds to the client, when it delivers the corresponding state change.

Zab?
- simple majority quorums on a proposal
- work with majority of servers survive

Request processing pipeline?
- To achieve high throughput, ZK tries to keep it full
- May have thousands of requests in different parts of the processing pipeline(TODO pipeline unit != one request? )
- Because state changes depend on previous state changes, Zab provides **stronger order guarantees** than regular atomic broadcast
  - order guarantee
  - all changes from previous leaders are delivered to an established leader before it broadcasts its won changes

Implement?
- Order: use TCP for transport, so meassage order is maintained by the network
- Leader Chosen: use Zab, who creates the transactions also proposes them
- Use log to keep track of proposals as the write-ahead log for the in-memory database, so that we do not have to write messages twice to disk(TODO understand write twice)

Zab doesn't maintain id of delivered message, not like raft's commitedIndex, so may deliver same message twice
- It's ok, since we use **idempotent** transactions, multiple delivery is acceptable, as long as in order
  - In fact, ZK use this feature when leader restart
    - ZK requires Zab to **redeliver** at least all messages that were delivered after the start of the last snapshot

## 4.3 Replicated Database

Replicas replicate what?
- Each replica has a copy in memory of the ZK state
- When a ZK server recovers from a crash, it needs to recover this internal state, replaying all delivered messages since the **start** of the snapshot
  - **start** is extremly IMPORTANT, since it combine a correct logic for later fuzzy snapshot

Fuzzy snapshot?
- We **do not lock ZK state to take the snapshot**
- We do DFS of the tree atomically reading each znode's data and meta-data and writing them to disk
  - Therefore our fuzzy snapshot may not match any real-time snapshot of the tree
    - newer sons and outdated parents? I guess
    - Snapshot generation reading on top nodes is paralled with updates on bottom nodes? I guess
- Here's where idempotent helps, all state changes including those during generation of fuzzy snapshot will be replayed
  - **Then the fuzzy snapshot's old-version nodes will be updated correctly**

## 4.4 Client-Server Interactions

How watch triggers from leader to client?
- I guess,server receives client's request$\rightarrow$ forward to leader $\rightarrow$ leader convert it to transaction and broadcast $\rightarrow$ all servers do that transactions, some one who connect to a client with "watch" locally stored will notify client
- When a server processes a write request, it also sends out and clears notifications relative to any watch that corresponds to that update.
  - I guess here "request" shall be "transaction"? Cause 
    - >All requests that **update**(TODO ps: Read is in local replica/nearest followers) ZooKeeper state are forwarded to the leader.
- Servers process writes in order(NO CONCURRENCY), so guarantee strict succession of notifications
- server handle notifications locally
  - only the server connecting with a client, tracks and tirggers notifications for that client
  - TODO: I guess, other followers receive leader's notification generated by watch will just ignore that?
    - ANSWER: Look below, watch is stored locally, therefore triggerd locally. Leader just simply broadcast an update, it's follower who find it trigger a watch
  
  When a client may miss a watch notification?
  - >Watches are maintained locally at the ZooKeeper server to which the client is connected. This allows watches to be light weight to set, maintain, and dispatch. When a client connects to a new server, the watch will be triggered for any session events.
  - >When you disconnect from a server (for example, when the server fails), you will not get any watches until the connection is reestablished. For this reason session events are sent to all outstanding watch handlers. Use session events to go into a safe mode: you will not be receiving events while disconnected, so your process should act conservatively in that mode.
  - >When a client reconnects, any previously registered watches will be reregistered and triggered if needed.

How the ZooKeeper deal with simple "READ"?
- Handled locally at each server

What's "Fast Read", will it lose consistency, getting stale results?
- The bare read itself is called "fast read", it's just memory actions so that's why ZooKeeper is fast in read-dominant situation
- Actually, it will lose consistency, you may get stale return result if you don't use $sync$(see below)
  - I think, Remember ZooKeeper is based on connection? The client will always see result generated by a same server, so the result will have Monotonic consistency
  - what we may have problem is only we may get stale results, but that result is still monotonic growing at least

How read requests are dealed by followers(who directly connected to clients), without loss of consistency?
- Depends whether your production needs precedence order, we can make some modify on READ
- Every read requests is processed and tagged with a $zxid$
  - $zxid$ corresponds to the last transaction seen by that server
- use ```sync```

What's ```sync```
- [Tradeoff between up2date & high performance, decided by user](https://stackoverflow.com/questions/5420087/apache-zookeeper-how-do-writes-work)
- it's asynchronous, ordered by the leader after all pending writes to its(TODO leader's?) local replica
- To make sure a ```read``` returns the latest updated value, a client calls ```sync``` followed by the ```read``` operation
- Enable any update, before sync, reflected to the ```read```

What will happen if client reconnect with another server?
- new server must be as up-to-date as the old server
- new server will check client's last $zxid$ and its self's $zxid$
- If new server is more stale, it won't reestablish the session until it has caught up
- Client is guaranteed to find another server as up2date, because of majority

# 5 Evaluation

# 6 Related work

# 7 Conclustions

