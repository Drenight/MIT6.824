# Lec9

Popular?
- Chain replication is fairly popular in many system

node fail?
- Head/Tail 
  - next/pre node take over
- Intermdediate
  - drop node, predecessor may need to resend info to new successor

# Abstract

What's CRAQ?
- Chain Replication with Apportioned Queries
- a distributed **object-storage** system, challenge sacrificing strong consistency for availability&throughput

Basic approach?
- an improvement on **Chain Replication**
- Gain both strong consistency + high throughput

✨✨✨✨✨Chain Replication versus Raft?
- Performance
  - Write
    - Raft: followers directly fed by leader
    - CRAQ: head only do one send
    - send on network are actually reasonably expensive
      - the load on a raft leader is HIGHER than in CRAQ
      - As clients increase, raft leader will hit limit faster, it's doing more work
  - Read
    - Raft read OP is also required to be processed by the leader
      - MYADVICE: Recall the "blank OP"
  - Specially, Raft doesn't need to wait for a lagging replica, while in chain one lagging node may lead write performance drop extremly(some body is installing a software)

What if second node can't talk to head, can he just take over?
- Not a good solution, may lead brain-split
- Actually this chain itself can't resist network partition
- It needs EXTERNAL AUTHORITY to resist network partition
  - need an aliveness monitor (a config MGR)
    - The config MGR always be Raft/Paxos/ZooKeeper

✨✨✨✨✨✨✨The usually complete set in your datacenter?
1. have a configuration manager based on RAFT/.., who is fault tolerent && doesn't suffer from split brain
   - this config MGR also serve to partition my data
2. Split my data over a bunch of chains, a room with thousands server
   - Refer to above versus, you may prefer raft here if you concern more at write's transient slowdown

# 1 Introduction

Application Scenario?
- Many online service, require *object-based* storage
- data is presented to app as entire units
- suppoted by key-value databases
- ✨object namespace is partitioned over many machines, each object is replicated several times

Object-based versus file-system storage?
- object stores are better suited for flat namespaces, such as in key-value DB, as opposed to hierarchical directory structures
- object stores simplify the process of supporting whole-object modifications
- need to provide consistency guarantee PER OBJECT instead of ALL STORAGE SYSTEM/OPERATIONS
  - TODO IGUESS granularity?

Cluster's basic organization?
- nodes are organized in a chain
- chain tail handles all read requests, and the chain head handles all write requests. 
- Writes propagate down the chain before the client
is acknowledged, thus providing a simple ordering of all
object operations—and hence strong consistency—at the
tail. 
- drawback: hotspot only read tail node

CRAQ's contributions?
1. Enable **any chain node to handle read operations while preserving strong consistency**, supporting load balancing across ALL NODES storing an object
   - Comparing with GFS, GFS only offer EVENTUAL CONSISTENCY
2. Besides strong consistency, support **Specifing maximum staleness acceptable, to get lower-latency reads**
3. Support a wide-area scenario across geographocally-diverse clusters

# 2 Basic System Model
- interface and consistency models, brief overview of the standard Chain Replication model comparing with CRAQ protocols

## 2.1 Consistency Model & Interface

Two simple primitives:
1. ```write(objID,V)```: udpate value
2. ```read(objID)```: retrieve the value $V$

Two main types of consistency, taken with respect to individual objects
1. Strong Consistency: CRAQ guarantees that all read and write operations to an object are executed in some sequential order, and that a read to an object always sees the **LATEST WRITTEN VALUE**
2. Eventual Consistency: In CRAQ implies that writes to an object are still in a sequential order on all nodes, but reads may return stale. If maintain session with fixed server can achieve monotonic read consistency

## 2.2 Chain Replication

How classic chain replication works?
- When a write operation is received by a node, it is propagated to the next node in the chain.
- Once the write reaches the tail node, it has been applied to all replicas in the chain, and it is considered committed.
- The tail node handles all read operations, so only values which are committed can be returned by a read.
  - the chain tail can trivially apply a total ordering over all operations.
    - To make sure which value to return

chain replication's features?
- write operations are cheaper than in other strong consistency
  - because multiple concurrent writes can be pipelined down the chain
- querying intermediate nodes could violate the strong consistency guarantee
  - read from different nodes could see different writes as they are in the process of propagating down the chain

## 2.3 Chain Replication, with Apportioned Queries

what CRAQ improves?
- CRAQ increase read throughput by allowing any node in the chain to handle read operations, while still providing strong consistency guarantees

CRAQ's main extensions?
1. a node in CRAQ stores multiple versions of an object, each includuing a monotonically-increasing version number and a flag $clean$ or $dirty$, initially $clean$
2. When a node receive a new version of an object(propagated down by previous node), the node appends this latest version to its list for the object
   - if the node not tail, mark the version as dirty, propagate write down
   - else node is tail, mark version clean(committed), send an ack backward let all nodes know it's clean now
     - TODO broadcast IGUESS? Cause still chain has delay
3. When a node receive that ACK, mark version clean, deleate all previous version
4. When node receives a read 
   - if lastest version is clean, return that value
   - else ask tail's last committed version number(a version query), node is guaranteed to store that version ob the object
5. read operations are SERIALIZED with respect to the tail

Can we save the use of flag "clean"?
- Yes, if a node has multiversion, it implies thoese are dirty.

## 2.4 Consistency Models on CRAQ


# 3 Scaling CRAQ
- scaling out CRAQ to many chains in/across datacenters

# 4 Extensions
- multi-object updates, multicast

# 5 Management and Implementation

# 6 Evaluation