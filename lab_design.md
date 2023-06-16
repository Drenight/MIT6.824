# lab2
我们学完6.824，目标是写出一个fault-tolerant的key/value存储系统

lab2是构建这个系统的基石--Raft，一个replicated state machine protocol，lab3在raft上写key/value服务，最后**shard**我的kv服务

一个replicated服务，achieves FT，通过存储完整的自身state on multiple replica servers. Replication 可以克服 some servers failures（crash/network），但带来了另一个问题：怎么不diverge

Raft这样克服diverge：所有节点，服从一个request sequence log
- 让每个节点看到的log都一样

每个replica按照log的顺序，把req输入本地状态机
- 核心思想

崩掉的机器回来后，我们把它bring its log up to date

只要有大部分机器还活着，集群就还能正常工作
- 数量不够，整个集群就不会有任何progress
  - 选不出新leader

lab2需要实现：
- 一组raft实例，通过RPC维护replicated logs
  - 给定index的log entry最终需要commit，at that point，我的raft需要把log entry送到**一个Larger的服务去运行**

## 2A
2A会起三个raft进程，要求实现
1. leader选举
2. leader心跳

要能保证在各种断网的测试下，始终保证0或1个leader
- 网络稳定的话，不failover一直是他
- 为啥断网？这个测试就是通过断网再连回来，模拟节点故障的

### 设计思路
创建raft实例后，起两个守护进程
- ```bkgRunningCheckVote```：检查集群leader状态
- ```bkgRunningAppendEntries```：leader心跳

```bkgRunningCheckVote```，频繁释放掉实例的锁，清空自己的leader，睡1s(+random bias 防分票)
- 这释放锁+1s的睡，是为了给别的raft实例发过来的心跳，让出优先级，允许他们更改自己的leader和voteFor
  - 心跳的频度需要设计的比1s高
- 这1s睡醒后，上锁检查leader/voteFor字段
  - 如果没人改过，本实例认为集群里没有leader并且自己没投过票，进入candidate流程
  - 如果改过，跳回函数开头继续睡, Reset & Sleep
- 进candidate流程，升term，并发请求所有其他实例，给自己投票
  - 因为自身的term大，如果自己的log最新，比自己term小的其他实例都会vote YES
    - **这个log最新还挺重要的，设想一个连不上leader的节点一直在increment自己的term，如果不限制log up2date，他会覆盖掉整个集群的**,限制了那个节点就会漏怯了
      - 这个机制在2A还没有实现，因为这个时候的log还都是空的，所以覆盖掉整个集群也不会出现任何问题，TODO往后写的时候需要回来更新下投票机制
  - 一个并发计票的设计细节，221024之前采用channel给投票线程输入结果，主线程并发请求完后一直循环读channel，拒绝/同意过半后跳出
    - 这个设计存在一个问题，如果一直收不到结果，主线程会阻塞在读channel，没法超时。
    - 虽然不知道，一直收不到结果这个状况怎么出现的，可能跟他具体RPC怎么处理断网挂钩，不过按道理我也没有实现竞选超时，本身设计就和论文不一样了
  - 221024后，改成单个变量计票，主线程一直读那个变量，投票线程跑出结果之后，直接抢raft的整个实例锁，改这个计票器
    - 失败率目前为止都是0，比之前test结果好多了
    - 新的设计遇到了一个go的经典并发问题
      - **在多线程中使用循环变量**
    - ```go test -run 2A -race``` 跑出了问题
      - race定义：
        - > A data race occurs when two goroutines access the same variable concurrently and at least one of the accesses is a write. 
      - TODO加上```-race```，整个进程发生了什么变化？为什么不加不会报错？
        - go对于map的并发会panic掉，但单个变量不会报错
        - 加上-race可以让单个变量的这种并发读写变得可见，可以发现存在这种读写
        - 整个进程的运行，没有变化
    - [Race Report Format](https://go.dev/doc/articles/race_detector)
    ```shell
    ==================
    WARNING: DATA RACE
    Write at 0x00c000326a10 by goroutine 94:
    ??()
        -:0 +0x10447fab8
    sync.(*Mutex).Lock()
        /opt/homebrew/Cellar/go/1.17.8/libexec/src/sync/mutex.go:74 +0x40
    _/Users/liuboyao/go/MIT6.824/lab1/src/raft.(*Raft).GetMutex()
        /Users/liuboyao/go/MIT6.824/lab1/src/raft/raft.go:297 +0xd88
    _/Users/liuboyao/go/MIT6.824/lab1/src/raft.(*Raft).bkgRunningCheckVote()
        /Users/liuboyao/go/MIT6.824/lab1/src/raft/raft.go:172 +0xd7c

    Previous read at 0x00c000326a10 by goroutine 140:
    reflect.Value.Int()
        /opt/homebrew/Cellar/go/1.17.8/libexec/src/reflect/value.go:1343 +0x1770
    fmt.(*pp).printValue()
        /opt/homebrew/Cellar/go/1.17.8/libexec/src/fmt/print.go:745 +0x171c
    fmt.(*pp).printValue()
        /opt/homebrew/Cellar/go/1.17.8/libexec/src/fmt/print.go:806 +0x1b98
    fmt.(*pp).printValue()
        /opt/homebrew/Cellar/go/1.17.8/libexec/src/fmt/print.go:806 +0x1b98
    fmt.(*pp).printValue()
        /opt/homebrew/Cellar/go/1.17.8/libexec/src/fmt/print.go:876 +0x11b0
    fmt.(*pp).printArg()
        /opt/homebrew/Cellar/go/1.17.8/libexec/src/fmt/print.go:712 +0xef0
    fmt.(*pp).doPrintf()
        /opt/homebrew/Cellar/go/1.17.8/libexec/src/fmt/print.go:1026 +0x370
    fmt.Fprintf()
        /opt/homebrew/Cellar/go/1.17.8/libexec/src/fmt/print.go:204 +0x5c
    fmt.Printf()
        /opt/homebrew/Cellar/go/1.17.8/libexec/src/fmt/print.go:213 +0xc8
    _/Users/liuboyao/go/MIT6.824/lab1/src/raft.(*Raft).RequestVote()
        /Users/liuboyao/go/MIT6.824/lab1/src/raft/raft.go:392 +0x44
    runtime.call32()
        /opt/homebrew/Cellar/go/1.17.8/libexec/src/runtime/asm_arm64.s:415 +0x70
    reflect.Value.Call()
        /opt/homebrew/Cellar/go/1.17.8/libexec/src/reflect/value.go:339 +0x98
    _/Users/liuboyao/go/MIT6.824/lab1/src/labrpc.(*Service).dispatch()
        /Users/liuboyao/go/MIT6.824/lab1/src/labrpc/labrpc.go:494 +0x3e0
    _/Users/liuboyao/go/MIT6.824/lab1/src/labrpc.(*Server).dispatch()
        /Users/liuboyao/go/MIT6.824/lab1/src/labrpc/labrpc.go:418 +0x218
    _/Users/liuboyao/go/MIT6.824/lab1/src/labrpc.(*Network).processReq.func1()
        /Users/liuboyao/go/MIT6.824/lab1/src/labrpc/labrpc.go:238 +0x70

    #这两个线程在哪里创建的:

    Goroutine 94 (running) created at:
    _/Users/liuboyao/go/MIT6.824/lab1/src/raft.Make()
        /Users/liuboyao/go/MIT6.824/lab1/src/raft/raft.go:544 +0x180
    _/Users/liuboyao/go/MIT6.824/lab1/src/raft.(*config).start1()
        /Users/liuboyao/go/MIT6.824/lab1/src/raft/config.go:209 +0x7d4
    _/Users/liuboyao/go/MIT6.824/lab1/src/raft.make_config()
        /Users/liuboyao/go/MIT6.824/lab1/src/raft/config.go:91 +0x708
    _/Users/liuboyao/go/MIT6.824/lab1/src/raft.TestReElection2A()
        /Users/liuboyao/go/MIT6.824/lab1/src/raft/test_test.go:69 +0x44
    testing.tRunner()
        /opt/homebrew/Cellar/go/1.17.8/libexec/src/testing/testing.go:1259 +0x198

    Goroutine 140 (finished) created at:
    _/Users/liuboyao/go/MIT6.824/lab1/src/labrpc.(*Network).processReq()
        /Users/liuboyao/go/MIT6.824/lab1/src/labrpc/labrpc.go:237 +0x190
    ==================
    ```
    - 查出来了，```fmt.Printf("%+v", rf.mu) //我猜是你小子报的race```，用+v读一把锁住的锁，会产生竞态

  - 睡1s后自身可能成为leader，这里的bug被修复了

## 2B 
2B的开发过程中，2A的test3没法通过，看到的现象是新选上的leader来不及发心跳，就被断网，上一轮leader仍然活跃
- 具体原因，目前猜测是代码变长，单轮AppendEntries变慢
- DONE或许我需要了解一下go test 怎么多次跑，这样省的盯着复现
- 现在的思路是，晋升leader后立刻触发一次AppendEntries，不知道有没有用，按道理也就100ms内该发了呀，难道是抢锁一直抢不到？
  - 把非leader 5ms循环check发心跳的逻辑去掉了，改成晋升后发一次，所有AE都100ms，减少了一部分test3的fail，还是有，继续debug
    - 非leader频繁5ms上锁看起来影响到成功率了
- 221030定位原因了，milliesecond误写成nanosecond，改成后撞见一个没release mutex的if分支，锁了两次，直接把tester卡住

有个点花了很久才想通：raft怎么同时做到
1. 拒绝低Term的AE
2. 拒绝不up-to-date的log的AE
- [stackoverflow](https://stackoverflow.com/questions/47568168/how-raft-follower-rejoin-after-network-disconnected)
  - 就是要在一台机器断网，带着高term和短log回来的时候，**让leader下台**
  - 然后让整个集群一直尝试选leader，直到有一个含整个log的机器，term增长到$>10$，整个集群恢复正常
  - >If RPC request or response contains term T > currentTerm: set currentTerm = T, convert to follower

关于应用传入信息：
- one()，给10s时间，在线的机器中需要有个leader来响应，消费掉这条cmd
- 测试中有绕过one直接发给离线机器的start的方法，看到不用confuse

### 221102，2B的backup测试点一直不稳定
- 看到的log是一直竞选不出leader
- 目前猜测是活锁，一直竞选不上leader

目前的开发效率感觉比较低下，一方面2B的一轮测试要花费超过1min，开多窗也需要人在屏幕前等到log分散开；
长时间的测试导致现在的开发工作流不合理：摇老虎机等bug复现，读冗长的log，推理自己tricky的实现，很浪费时间

> most of your bugs will be a result of not faithfully following Figure 2.

接下来准备做两个事情
1. 按照几个课程建议文档，重构下代码，把诸如hasleader这种自己很tricky的实现，换成朴素的文档推荐的实现，降低debug心智成本
2. > We will be talking about Figure 2 a lot in the rest of this article. It needs to be followed **to the letter**.别自己瞎搞了，提供正确性的系统，很容易出错
   - [Students' Guide](https://thesquareplanet.com/blog/students-guide-to-raft/)
     1. >Many of our students assumed that heartbeats were somehow “special”; that when a peer receives a heartbeat, it should treat it differently from a non-heartbeat AppendEntries RPC. In particular, many would simply reset their election timer when they received a heartbeat, and then return success, without performing any of the checks specified in Figure 2. This is extremely dangerous. By accepting the RPC, the follower is implicitly telling the leader that their log matches the leader’s log up to and including the prevLogIndex included in the AppendEntries arguments. Upon receiving the reply, the leader might then decide (incorrectly) that some entry has been replicated to a majority of servers, and start committing it.
     2. appendEntries不能truncate然后append args的全部，因为这个req可能过时了，后面有更新的log    
   - [Raft Locking Advice 锁文档](http://nil.csail.mit.edu/6.824/2021/labs/raft-locking.txt)
     1. 推敲所有现有的锁逻辑 
     2. 重获锁后，校验是否有状态变化
   - [Raft Structure Advice 结构文档](http://nil.csail.mit.edu/6.824/2021/labs/raft-structure.txt)
     1. hasleader->结构体内的lastHeartBeatTS
     2. 开独立线程,去applyMsg,因为写chan可能会阻塞
3. 优化下测试方法，搞明白多窗同时开会不会有问题，注释单点测backup之类

2B的测试通过了1000/1000，有个Fast Backup的测试点需要对快速匹配prev做优化，不做很容易fail掉
- 自己的实现是每次失配，nextIndex/=2，想模仿TCP那个拥塞控制的1，2，4，8增长
  - 这个估计之后会改，折半发太多了，不过如果真想1248得记录一些字段来实现
- Lecture7讲的做法：每次回退一整个term

## 2C
修正了follower在AppendEntries仍然清掉votedFor的问题
- 如果清掉会导致同一个term产生多个leader


Unreliable那边的测试会爆问题
- 修改了majority的逻辑，怀疑之前的O(log(n))二分有问题，现在是O(n)的
  - 可能会拉低AE频率，准备优化下这边
- 目前是把log下降从```/=2``` 变成 ```=sqrt```

## 2C-3A
似乎隔得太久，更新了vscode上一些东西导致package那一行一直gopls的问题，以及有一些之前就有的假阳性undeclear
- 整个项目搬出GOPATH了，向gomod兼容吧
- 这个项目本身没有go.mod，一旦创建了就会不支持../这样的import，不知道MIT新的代码是不是翻新了，还是忍着假阳性写代码
  - 又或是我哪里理解错了
  - check过了，22版的把所有../的引用全换成6.824/了，也有go.mod，羡慕死了，要怪就怪自己拖太久了吧
- goland甚至直接不让../哪怕我没创建go.mod

## 3A
整理下raft与kv的交互
> as each Raft peer becomes aware that successive log entries are
  committed, the peer should send an ApplyMsg to the service (or
  tester) on the same server, via the applyCh passed to Make(). set
  CommandValid to true to indicate that the ApplyMsg contains a newly
  committed log entry.
  
> in Lab 3 you'll want to send other kinds of messages (e.g.,
  snapshots) on the applyCh; at that point you can add fields to
  ApplyMsg, but set CommandValid to false for these other uses.

1. 每个kvsrv 有个 Raft peer
2. Clerk把put/app/get RPC发给leader Raft绑定到的kvsrv
3. kvsrv把put/app/get下传给raft, raft集群维护3op的log
4. 所有kvsrv执行底层raft的3oplog
    - 这里要起一个线程估计，监听raft的applyMsg
5. kvsrv commit了一个op，需要响应clerk的RPC汇报结果

上面是给的提示，下面是一些自己的想法
1. 每个kvsrv开一个线程监听applyMsg Chan
2. sync.Cond
   - Done 目前正在尝试replace掉raft的cond改用chan，因为[官方文档](https://pkg.go.dev/sync#Cond.Broadcast)推荐
    >For many simple use cases, users will be better off using channels than a Cond (Broadcast corresponds to closing a channel, and Signal corresponds to sending on a channel).
3. clerk那里给个prefer？防止重新探查leader？
4. 考虑给kv一个本地的队列，维护收到的req，这样每次msg chan收到的东西拿来跟队列头比较一下时序就可以做到响应了，而不需要fan out
   - 之前考虑过fan out，就是多个req同时监听一个chan，但想想似乎req有时序的，队列就可以
   - 新想法，给kv一个map，维护(index,term)->chan

### 0610
开始写3A with failures，面临的问题：
1. clerk在收到success回复前可能发多次RPC，需要保证这些RPC不会引发多次的执行
   - 例如：一个leader commit了一个entry，然后立刻挂掉，导致这个commit没有response
   - 这个似乎需要复习下：raft怎么处理低term的entry？不让commit吗？
     - >Raft never commits log entries from previous terms by counting replicas
     - >The restriction ensures that the leader for any given term contains all of the entries committed in previous terms
  - 其实不止这种（没响应），还譬如leader更替了，不能还以为自己是leader响应个success
    - 这个真的有问题吗？srv感知到leader更替了，肯定是没阻塞在监听chan啊，那要么成了要么没成呗
    - >For example, you may have been the leader when the client initially contacted you, but someone else has since been elected, and the client request you put in the log has been discarded. Clearly you need to have the client try again, but how do you know when to tell them about the error?
      - 参考了TA的student guide，似乎只是个不严重的问题，无非就是优化掉超时和让client感知到新leader？

作业的提示：
1. 让server发现自己不是leader：
   1. 相同index出现了别的request
   2. 系统term变更了
2. clerk加个prefer server，加速，上面提过
3. unique id每个RPC，来取得执行仅一次
    - 在哪落地检测呢？ 
4. duplicate检测，要尽量快速释放服务器的内存
    - >Your scheme for duplicate detection should free server memory quickly, for example by having each RPC imply that the client has seen the reply for its previous RPC. It's OK to assume that a client will make only one call into a Clerk at a time.

有个想法
- 每个client维护一个递增的请求号，在收到commit的时候增
  - 似乎可以直接用个uuid？不会有4,5,6需要用到4的？
    - 感觉可行，不去拒绝apply，而是响应新请求，既然现在请求是用clientID找的？
    - 这样所有机器都一致地执行log，特定接到req的机器拒绝start新请求就行
      - 这个拒绝有点时序问题？或者还是所有机器记录每个client最后apply的req
- server维护一个map，client->apply过的最大请求号

担心的点
- replay咋办呢？这台server挂掉的话
  - 不是不行吧，重跑整个log继续维护最大请求号？snapshot有点担心
- client挂了咋办，不让挂？每次写磁盘？
  - 会不会有种实现就是让客户端的请求为-1的时候表示一定接受啊？

用logIndex标识请求，apply相应chan的问题
- 这台机器start了，开始监听那个chan
- 这台机器lose leader，被覆盖log，新leader在那个log位置写了个别人的request
- 这台机器的apply线程发现那个log位置的map里有个老chan，往里写
- **这台机器会响应一个别人的response给原client**

### 0615
0610的问题应该想清楚了
1. 还是index2chan，这样可以select同index非同client的时候响应leaderChange
2. 每台replica维护lastApply，client2requestID，filter掉redundant

目前test3出了份log，明天继续调，现象是同一个log始终被报applied但是没成功被client ACK