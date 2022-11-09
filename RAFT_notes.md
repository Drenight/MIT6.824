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

Roll Back 一次1条log会非常耗时
- 设想一台follower崩了重启，然后换了个leader
  - 新leader需要从他的log尾一个RPC一条地往回匹配follower的空log
- 我在lab的实现里是/=2，lec7里一个同学提到了binary search，跟这个差不多，都是发更多的log

Fast Backup 快速备份
- 让follower发送足够的信息，让leader回跳整个term

### persist and volatile
- Crash and reboot, 需不需要从磁盘加载的数据

为啥不直接从空的服务器reboot？譬如这台机器遭遇磁盘损坏了，这种从0恢复到up-to-date的能力肯定是要有的
- 因为依赖这种能力的前提，是整个集群不会同时shutdown，考虑同时断电。为了这种极端情况，我们最好还是能有个磁盘，长期持久化存储。

persist哪些？
- log
- currentTerm
- votedFor

怎么确保写进磁盘？
- fsync(fd)，让进程阻塞直到成功写入，只是调用write(fd,_)并不确保写进去

写磁盘太慢了，每次更新log/currentTerm/votedFor都写不现实，怎么办？
- 10ms级别的读写，要等磁盘转到合适的位置，远比一次RPC耗时长
  - 每次都写磁盘的做法叫 Synchronous disk update
    - 例如我手上电脑的文件系统，为了保全我的写入，都是synchronous disk update的
- 可以把落盘，推迟到每次与外界交互
  - 这就引入了batch一批client的请求，一起处理一起响应的优化，这样就不用一直写磁盘了

Batch trick
- 积累一堆client的请求，不响应直到这一批都处理完
  - reply意味着promise to commit
 
why不存比如lastApplied?
- 机器reboot，从leader/followers那里得知集群的commitIndex，replay log里该commit的就行
- 一个缺陷是这样非常慢，这也就引出了下一个topic，log compaction

快照
- 绑定一个logIndex作为版本号，之前的log就不需要了
  - 如果有follower的log断在这个index之前怎么办？
     1. leader根据nextIndexMap永不删除全有的？
        - 不太好，有个follower挂一周，你就一周不能做快照
     2. Install Snapshot RPC, AE发现找到log头了，发送自己的最新快照和自己的log
- 生成快照的功能是APP做的，raft在需要的时候会去调用，因为只有应用（那个key-value服务）理解他的表是什么意思
- 收到的快照可能是个stale的，install得小心

Linearizeability
- 线性一致（严格一致，原子一致），整个集群运作就像一台，永远不crash的机器，很强的一致性
- 定义：execution history is liniable if $\exists$ order of OPs that matches REAL for NON-CONCURRENCY. Each read sees MOST recent write.
- 读不该读到stale数据

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

# 7 Log compaction
Long log consumes $\rightarrow$ snapshot compact
- time to replay
- storage space

why not create snapshot only by leader & send to follower?
- two disadvantages:
  1. network bandwidth waste, slow snapshotting process
      - follower has enough knowledge to build its own snapshot
  2. make leader's implementation complex, busy with sending info

Snapshot Frequency?
- Fast: waste disk bandwidth and energy
- Slow: risk exhausting storage capacity & replay time grows
- Advice: a fixed bytes limit
  - if significantly > expected size of snapshot:
    - disk bandwidth for snapshotting will be small

# 8 Client interaction
- Talk about issues apply to all consensus-based system:
  - How client find cluster's leader?
  - How Raft supports linearizable semantics?

How client find cluster's leader?
- Talk to random server, that server will guide to the leader

What's linearizable semantics?
- every operation appears to execute instantaneously, exactly once.
- no stale reply!

How to make sure every operation run exactly once?
- When will run twice or more? Example?
   > for example, if the leader
   crashes after committing the log entry but before responding to the client, the client will retry the command with a new leader, causing it to be executed a second time.
  - Solution?
      >  The solution is for clients to assign unique serial numbers to every command. Then, the state machine tracks the latest
      serial number processed for each client, along with the associated response. If it receives a command whose serial
      number has already been executed, it responds immediately without re-executing the request.

- Contact stale leader, read returns stale result, how to solve?
   1. >First, a leader must have the latest information on
   which entries are committed. The Leader Completeness
   Property guarantees that a leader has all committed entries, but at the start of its term, it may not know which
   those are. To find out, it needs to commit an entry from
   its term. Raft handles this by having each leader commit a blank no-op entry into the log at the start of its
   term. 
   2. > Second, a leader must check whether it has been deposed before processing a read-only request (its information may be stale if a more recent leader has been elected).
   Raft handles this by having the leader exchange heartbeat messages with a majority of the cluster before responding to read-only requests.

A better explaination of why using blank op
> The no-op text at the end of Section 8 is talking about an optimization
in which the leader executes and answers read-only commands (e.g.
get("k1")) without committing those commands in the log. For example,
for get("k1"), the leader just looks up "k1" in its key/value table and
sends the result back to the client. If the leader has just started, it
may have at the end of its log a put("k1", "v99"). Should the leader
send "v99" back to the client, or the value in the leader's key/value
table? At first, the leader doesn't know whether that v99 log entry is
committed (and must be returned to the client) or not committed (and
must not be sent back). So (if you are using this optimization) a new
Raft leader first tries to commit a no-op to the log; if the commit
succeeds (i.e. the leader doesn't crash), then the leader knows
everything before that point is committed.

# 9 Implementation and evaluation
- Three topic to evaluate Raft

TLA+ specification language

## 9.3 Performance
- most important: replicating new log entries

# 10 Related work

# 11 Conclusion
- Challange Paxos's understandability