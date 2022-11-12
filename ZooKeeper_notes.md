[kafka replace zk with raft, why?](https://www.confluent.io/blog/why-replace-zookeeper-with-kafka-raft-the-log-of-all-logs/)
- TODO, system rely on system $\rightarrow$ system rely on inner algorithm?

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

## 4.1 Request Processor


ZK's DB?
- in-memoery
- each stores maximum 1MB data by default

# 5 Evaluation

# 6 Related work

# 7 Conclustions