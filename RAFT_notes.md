# 1 Introduction
Consus algorithms: allow a collection of machines to work as a coherent group that can survive the failures of some of its members

Raft's novel features:
1. Strong leader
   - e.g. log entries only flow from lead to other servers
2. Leader election 
   - use randomized timers to elect leaders, attached to heartbeats
3. Membership changes:
   - a new joint consensus approach, majorities of two different config overlap

# 2 Replicated state machines
used to solve a variety of fault tolerance problems in distributed system
- e.g. GFS

# 3 What's wrong with Paxos
two significant drawbacks:
1. exceptionally difficult to understand
2. no good foundation for building practical implementations

# 4 Designing for understandability

# 5 The Raft consensus algorithm
a distinguished leader, complete responsible for managing the replicated log, who do:
1. accept log entries from clients
2. replicate log entries to other servers
3. tell servers when it is safe to apply log entries to their state machines

Raft decomposes the consensus problem into three subproblems
1. Leader election(5.2)
2. Log replication(5.3)
3. Safety(5.4, 5.2)

## 5.1 Raft basics
- Raft cluster contains several servers, usually 5

at any time each server is in one of three states:
1. leader
   - handle all client req, followers redirect clients' req to leader
2. follower
   - passive, issue no req, simply respond toreq from leaders and candidate
3. candidate
   - used to elect leader (5.2)
- in nomal operation, only one leader & others all followers

Raft divides time into terms, of arbitrary length, numbered with consecutive integers
- begins with **election**
  - one or more candidates attempt to become leader, win then serve for the rest of the term
  - if split vote, the term will end with no leader, then a term will begin shortly

different server may observe different loss, such as time bias, or even loss of elction, loss of entire term

Terms act as a logical clock in Raft, help detect stale info like stale leader

Each server store a **current term** number, and will exchange it when communicate with each other, smaller->**update**

- [ ] what means by "update" ?

- leader or candidate find it's smaller, revert to follower state
- server reject a req with a stale term number

Raft servers communicate using RPC, basic consensus only require two kinds:
- RequstVote RPCs: election(5.2)
- AppendEntries RPCs: replicate log entries, heartbeat(5.3)

## 5.2 Leader election
- A server remains in follower state as long as it receives valid RPCs from le or can
- leader sends period heartbeat to maintain its authority
- over a period time without communication, it assumes no viable leader and begin an election

To begin an election: 
1. increment its term and transitions to candidate state
2. vote for itself and issue RequestVote RPCs

candidate status until one below:
1. win the elction
   - if receive votes from a majority of the servers
2. another server establishes itself as leader
   - receive an AppendEntries RPC claiming to beleader, evaluate its term
3. time exceed with no winner
    - randomize election timeouts,150-300ms, spread out the servers, reduce possiblity of split vote
    - rank brings filibuster in progress of election

## 5.3 Log replication
client request 
1. leader appends command to its log as a new entry
2. leader issues AppendEntries RPCs in parallel to each of the other servers to replicate the entry
3. **after entry safely replicated**, leader applies the entry to its state machine, return result to client
4. if followers crash or run slowly, or network lost, leader retries **indefinitely**, may be even after it responds to client, until all followers store all log entries
   - [ ] as shown in 5.4 figure8, once chosen be leader again it will try step2, after it crash???

- each log has a term number and log index, which is used to detect inconsistencies between logs, (x<-0, y<-1 can be in the same term cause performd in the same term, and different index)

> committed entry: leader decides it's safe to apply to state machines, durable, will be executed by all available state machines, once the leader replicated it on a majority of servers (why only need half? Once committed successfully, later leader will among these majority, those in minority wont get vote because vote comparing in 5.4.1)

  - leader keeps track of the highest index it knows to be committed, and will include that index in future AppendEntries RPCs & heartbeat
    - once followers learns a log entry is committed, it apply to its local
  

**Log Matching Property**: if two entries in different logs have the same index and term, then they
  1. store the same command
      - leader create max one entry with one given log index in one given term
  2. the logs are identical in all preceding entries
      - consistency check: leader send AERPC with fore index&term in its log, follower refuse if dont match

> To bring a follower’s log into consistency with its own,
the leader must find the latest log entry where the two
logs agree, delete any entries in the follower’s log after
that point, and send the follower all of the leader’s entries
after that point
   - happen in response to consistency check by AE_RPCs
   - leader maintains **nextIndex** for each follower, the next log entry leader will send to him
      - follower refuse, leader decrements it by one until accept

## 5.4 Safety
> However, the mechanisms
described so far are not quite sufficient to ensure that each
state machine executes exactly the same commands in the
same order. For example, a follower might be unavailable
while the leader commits several log entries, then it could
be elected leader and overwrite these entries with new
ones; as a result, different state machines might execute
different command sequences.
   - the example is exactly what my concern is so far
   - so we need to add restriction on election, by following sth discussed later 
   

### 5.4.1 Election restriction

How to restrict leader contains all entries  **committed** in previous terms?

- candidate must have contacted at least majority of cluster 

  - => every committed entry present in at least one of those servers (*1 impossible none, the previous leader must contacted one of them, to made sure that entry committed)

    - ==> if candidate log at least as up2date as any other in majority, it will hold all committed entries

whether to vote YES?
- candidate include its log info in RV_RPC, voter compare to its own log to see who is more up2date

> up2date: If the logs have last entries with different terms, then
the log with the later term is more up-to-date. If the logs
end with the same term, then whichever log is longer is
more up-to-date

### 5.4.2 Committing entries from previous terms

> If a leader crashes before committing an entry, future leaders will attempt to finish replicating the entry.
- [ x ] how future lead try means?
  - if leader crashes just before commit, then the last entry must have been received in majority, and the next leader will have that entry

> However, a leader cannot immediately conclude that an entry from a previous term is **committed** once it is stored on a majority of servers.
   - Paper Figure 8, old term may be covered 

   <span style="background-color: #FFFF00">
   This "committed" focus on the meaning of "durable", not means "being done"
   </span>

if previous leader crash before commit, later leader with low version may overwrite previous terms entry in cluster, How to deal?
   > Raft never commits log entries from previous terms by counting replicas
- dont believe it will be durable (and apply to your state machine) because it's on majority's records, if it's from history terms
  - Figure 8, don't commit(apply to state machine) old term's record.

### 5.4.3 Satety argument
Leader Completeness Property:
- committed log will be definitely in future leader's log
- proven by suppose and find contradicitions: leaderT commits a log, leaderU is the first leader who does not have it

State Machine Safety Property
- popularized by upper property, scope from state machine
- if a server has applied a log entry at a given
index to its state machine, no other server will ever apply a
different log entry for the same index

## 5.5 Follower and candidate crashes
- simpler than leader crashes

Raft handles these failures by retrying indefinitely, since Raft RPCs are idempotent, be discarded

## 5.6 Timing and availability
> safety must not depend on timing. System must not produce incorrect results just because some event happens more quickly or slowly than expected