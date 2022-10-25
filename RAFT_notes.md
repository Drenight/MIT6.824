# MIT Syllabus
MR,GFS,FTVM的共同点？
- MR replicate计算，但依赖单点master
- GFS replicate 数据，依赖单点master选择primary
  - primary告知其他replica绝对位置
- FTVM replicate 服务，依赖外部test-and-set选择primary
- RAFT是没有单点故障的

脑裂怎么产生的，为什么会造成damage？
- 网络隔断partition，各自认为无leader
- 集群diverge，两次read数据不一致

基于raft的k/v系统时序？
1. client请求leader的k/v层
2. leader把req加进自己的log
3. leader发appendEntries，让followers把req加进自己的log
4. followers加进了log的话，就汇报给leader成功，leader统计过半
5. 过半followers汇报log写入的话，leader认为这条log是"commited"的
   - > committed means won't be forgotten even if failures
6. leader执行这条req，汇报给client，操作完成
7. leader在心跳里夹带"执行"命令，告诉follower把log里这个操作执行掉
8. follower执行

为什么是"基于log一致"？
- log的顺序唯一确定了，req的执行顺序
- 保证所有replica像一致的状态机state machine

结合下DDIA，这里保证什么样的一致性？
- 复习：DDIA讲了：
  - 最终一致
    - 纯异步系统，lag最终会catch up
    - 异步replication**基本**一致性
  - read-after-write一致
    - 写完立刻读，能读到新数据
    - 让用户安心
    - 自己的读去leader/Track更新1min内读leader/客户最新ts筛选follower
  - Monotonic Reads一致
    - 读的数据版本不会回退
    - 读同一个follower，故障转发
  - Consistent Prefix一致
    - 读一组相关的操作，不会乱序
    - Track相关操作的前后顺序
- 首先最终一致肯定是有的，其次因为整个集群基于log，前缀一致读也是有的
- 另外俩都不保证

term和leader的映射关系？
- >new leader -> new term
- >a term has at most one leader; might have no leader
  - 一个term分票了，下一轮candidate又会increment term

follower roll-back是什么机制？
- each live follower deletes **tail of log** that **differs from leader**, copy leader's 'correct memory'

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
