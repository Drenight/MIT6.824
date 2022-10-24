package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"../labrpc"
)

var voteExpire = int64(200)

// import "bytes"
// import "../labgob"

//
// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make(). set
// CommandValid to true to indicate that the ApplyMsg contains a newly
// committed log entry.
//
// in Lab 3 you'll want to send other kinds of messages (e.g.,
// snapshots) on the applyCh; at that point you can add fields to
// ApplyMsg, but set CommandValid to false for these other uses.
//
type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int
}

//
// A Go object implementing a single Raft peer.
//

type LogEntry struct {
	Term  int64
	Index int64
}

type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]
	dead      int32               // set by Kill()

	// Your data here (2A, 2B, 2C).
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.
	hasLeader bool
	term      int64 //term ME kept in logs
	votedFor  int   //leader voted for in this term, -1 for NULL, self means is candidate

	isLeader bool
	logSlice []LogEntry
}

func (rf *Raft) beLeader() {
	rf.isLeader = true
	fmt.Printf("**I, %v am leader of Term %v", rf.me, rf.term)
}

func (rf *Raft) bkgRunningCheckVote() {
	for { // test for starting leader vote
		if rf.killed() {
			fmt.Printf("%v this is dead\n", rf.me)
			return
		} else { //alive, examine whether receive hb from leader
			//fmt.Printf("try lock %v ", rf.me)
			rf.GetMutex()
			//fmt.Printf("lock %v!", rf.me)
			if rf.isLeader {
				rf.ReleaseMutex()
				fmt.Printf("release lock %v as leader\n", rf.me)
				time.Sleep(time.Second)
				//fmt.Printf("I, %v, is a leader\n", rf.me)
				continue
			}
			rf.hasLeader = false
			rf.ReleaseMutex()
			fmt.Printf("release lock %v, reset my hasLeader!\n", rf.me)
			r := rand.New(rand.NewSource(time.Now().UnixNano()))
			time.Sleep(time.Millisecond*time.Duration(voteExpire) + time.Millisecond*time.Duration(r.Int()%199))

			//fmt.Printf("Try %v...", rf.me)
			rf.GetMutex()
			//fmt.Printf("Ok %v!", rf.me)
			if rf.hasLeader || (rf.votedFor != -1 && rf.votedFor != rf.me) { //has get new leader OR has voted in this term
				fmt.Printf("I, %v already has leader or voted, voted for is %v ", rf.me, rf.votedFor)
				rf.ReleaseMutex()
				fmt.Printf("Release lock %v!\n", rf.me)
				continue
			} else { //be candidate, start to sendRequestVote

				//todo
				//If AppendEntries RPC received from new leader: convert to follower
				//If election timeout elapses: start new election

				fmt.Printf("I, %v, think there's no leader \n", rf.me)

				rf.term++
				rf.votedFor = rf.me

				args := RequestVoteArgs{
					Term: rf.term,
					Id:   rf.me,
				}
				//replys := make([]RequestVoteReply, len(rf.peers))
				cntYes := 1
				cntNo := 0
				//var wg sync.WaitGroup

				//start rpc and release lock
				//expect AERPC lock modify all, which is more prior
				//args is locked value, wont change result of RPCs
				rf.ReleaseMutex()
				fmt.Printf("Release lock %v!\n", rf.me)

				//wg.Add(len(rf.peers) - 1)
				c := make(chan int)

				//allCnt := 0
				startTime := time.Now().UnixNano()

				fmt.Printf("%v!!!\n", len(rf.peers))

				for i := 0; i < len(rf.peers); i++ {
					fmt.Printf("%v %v!\n", i, rf.me)
					if i == rf.me {
						continue
					}
					tmp := i //classic bug, using iteration args with multithread
					go func(i int) {
						fmt.Printf("%vxxx\n", i)
						reply := RequestVoteReply{}
						rf.sendRequestVote(i, &args, &reply, c /*, &wg*/)
						if reply.VoteAsLeader {
							rf.GetMutex()
							cntYes += 1
							rf.ReleaseMutex()
						} else {
							rf.GetMutex()
							cntNo += 1
							rf.ReleaseMutex()
						}
					}(tmp)
				}

				for true {
					time.Sleep(time.Millisecond * time.Duration(10))
					rf.GetMutex()
					if cntYes > len(rf.peers)/2 { //odd
						rf.beLeader()
						rf.votedFor = -1 //?不一定要改，通过高term覆盖就可以了？
						rf.ReleaseMutex()
						break
					}
					if cntNo > len(rf.peers)/2 {
						rf.votedFor = -1
						rf.ReleaseMutex()
						break
					}
					if time.Now().UnixNano()-startTime > 3000 {
						rf.votedFor = -1
						rf.ReleaseMutex()
					}
				}
				fmt.Printf("**I am %v, get %v votes, and %v noVotes, total is %v, sum less means expire!\n", rf.me, cntYes, cntNo, len(rf.peers))

				/*
					for true {
						if cntYes > len(rf.peers)/2 { //get vote from majority
							rf.GetMutex()
							rf.beLeader() //beleader
							rf.votedFor = -1
							rf.ReleaseMutex()
							time.Sleep(time.Second) //leader rest
							break
						} else if allCnt == len(rf.peers)-1 {
							rf.GetMutex()
							rf.votedFor = -1
							rf.ReleaseMutex()
							break
						} else if cntNo >= len(rf.peers)/2 {
							rf.GetMutex()
							rf.votedFor = -1
							rf.ReleaseMutex()
							break
						} else if time.Now().UnixNano()-startTime > 3000 {
							rf.GetMutex()
							rf.votedFor = -1
							rf.ReleaseMutex()
							break
						}
						x := <-c	//怀疑半年前这样写，会阻塞住一直不触发3000ms退出，尝试重写 221024，尝试成功，显著降低失败率，我好牛逼
						allCnt++
						if x == 1 {
							cntYes++
						} else {
							cntNo++
						}
					}
					fmt.Printf("**I am %v, get %v votes, and %v noVotes, total is %v\n", rf.me, cntYes, cntNo, len(rf.peers))
				*/

				//fmt.Printf("After Lock %v waiting", rf.me)
				//wg.Wait()
				//fmt.Printf("Waitgroup locking %v Done\n", rf.me)
				//if leader comes back, the cnt must be <(?)

				//fmt.Printf("did I wait?")
				/*
					for i := 0; i < len(rf.peers); i++ {
						if replys[i].VoteAsLeader {
							fmt.Printf("voteFrom%v ", i)
							cnt++
						}
					}
				*/

				/*
					for true {
						if allCnt == len(rf.peers)-1 {
							break
						}
						_ = <-c
						allCnt++
					}
				*/

				close(c)
				fmt.Printf("**I am %v, all vote done\n", rf.me)
				//rf.ReleaseMutex()
			}
		}
	}
}

func (rf *Raft) bkgRunningAppendEntries() {
	for {
		if rf.killed() {
			fmt.Printf("%v this is dead\n", rf.me)
			return
		} else {
			time.Sleep(time.Millisecond * time.Duration(10))
			rf.GetMutex()
			if rf.isLeader {
				args := AppendEntriesArgs{
					Term: rf.term,
				}
				fmt.Printf("$$I, %v, is leader\n", rf.me)
				rf.ReleaseMutex()
				reply := AppendEntriesReply{}
				var wg sync.WaitGroup
				//wg.Add(len(rf.peers) - 1)
				for i := 0; i < len(rf.peers); i++ {
					if i == rf.me {
						continue
					}
					go rf.sendAppendEntries(i, &args, &reply, &wg)
				}
				time.Sleep(time.Millisecond * time.Duration(90))
				//wg.Wait()
			} else {
				rf.ReleaseMutex()
			}
		}
	}
}

func (rf *Raft) GetMutex() {
	rf.mu.Lock()
}
func (rf *Raft) ReleaseMutex() {
	rf.mu.Unlock()
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {
	if rf.killed() {
		fmt.Println("HAAAAA")
	}
	var term int
	var isleader bool
	// Your code here (2A).
	//fmt.Printf("seems I am stuck to get lock... %v\n", rf.me)
	rf.GetMutex()
	//fmt.Printf("oh I am not to get %v\n", rf.me)
	term = int(rf.term)
	isleader = rf.isLeader
	rf.ReleaseMutex()
	return term, isleader
}

//
// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
//
func (rf *Raft) persist() {
	// Your code here (2C).
	// Example:
	// w := new(bytes.Buffer)
	// e := labgob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// data := w.Bytes()
	// rf.persister.SaveRaftState(data)
}

//
// restore previously persisted state.
//
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (2C).
	// Example:
	// r := bytes.NewBuffer(data)
	// d := labgob.NewDecoder(r)
	// var xxx
	// var yyy
	// if d.Decode(&xxx) != nil ||
	//    d.Decode(&yyy) != nil {
	//   error...
	// } else {
	//   rf.xxx = xxx
	//   rf.yyy = yyy
	// }
}

//
// example RequestVote RPC arguments structure.
// field names must start with capital letters!
//
type RequestVoteArgs struct {
	// Your data here (2A, 2B).
	Id   int
	Term int64
}
type AppendEntriesArgs struct {
	Term int64
}

//
// example RequestVote RPC reply structure.
// field names must start with capital letters!
//
type RequestVoteReply struct {
	// Your data here (2A).
	VoteAsLeader bool
}
type AppendEntriesReply struct {
}

//
// example RequestVote RPC handler.
//
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (2A, 2B).
	//fmt.Printf("I am called by %v, going to LOCK %v", args.Id, rf.me)
	rf.GetMutex() //BUG DEAD LOCK?
	//fmt.Printf("I am called by %v, LOCKED %v", args.Id, rf.me)

	fmt.Printf("I am %+v, args is %+v\n", rf, args)

	if rf.term < args.Term && (rf.votedFor == -1 || rf.votedFor == args.Id) { //todo,split vote?
		rf.votedFor = args.Id
		reply.VoteAsLeader = true
		fmt.Printf("I, %v, vote %v YES\n", rf.me, args.Id)
	} else {
		reply.VoteAsLeader = false
		fmt.Printf("I, %v, vote %v NO\n", rf.me, args.Id)
	}

	rf.ReleaseMutex()
	fmt.Printf("I am called by %v, going to RELEASE %v\n", args.Id, rf.me)
}
func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.GetMutex()
	if rf.term > args.Term {
		rf.ReleaseMutex()
		return
	}

	rf.isLeader = false
	rf.hasLeader = true
	rf.votedFor = -1
	//todo, valid check
	rf.term = args.Term
	rf.ReleaseMutex()
}

//
// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// The labrpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost.
// Call() sends a request and waits for a reply. If a reply arrives
// within a timeout interval, Call() returns true; otherwise
// Call() returns false. Thus Call() may not return for a while.
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost reply.
//
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return.  Thus there
// is no need to implement your own timeouts around Call().
//
// look at the comments in ../labrpc/labrpc.go for more details.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
//server int, args *RequestVoteArgs, reply *RequestVoteReply
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply, c chan int /*, wg *sync.WaitGroup*/) bool {
	fmt.Printf("I am calling to server %v, by %v\n", server, args.Id)
	//defer wg.Done()
	defer fmt.Printf("requestVote's %v is ok!", server)
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply) //TODO, kick off a new routine with expiration?
	fmt.Printf("I have called to server %v, by %v,ok is %v\n", server, args.Id, reply.VoteAsLeader)
	if reply.VoteAsLeader {
		return true //c <- 1
	} else {
		return false //c <- 0
	}
	//if !ok {
	//	fmt.Printf("not ok call to %v\n", server)
	//}
	return ok
}
func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply, wg *sync.WaitGroup) bool {
	//defer wg.Done()
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	fmt.Printf("$$Leader heartBeat from %v to %v\n", rf.me, server)
	return ok
}

//
// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election. even if the Raft instance has been killed,
// this function should return gracefully.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
//
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	index := -1
	term := -1
	isLeader := true

	// Your code here (2B).

	return index, term, isLeader
}

//
// the tester doesn't halt goroutines created by Raft after each test,
// but it does call the Kill() method. your code can use killed() to
// check whether Kill() has been called. the use of atomic avoids the
// need for a lock.
//
// the issue is that long-running goroutines use memory and may chew
// up CPU time, perhaps causing later tests to fail and generating
// confusing debug output. any goroutine with a long-running loop
// should call killed() to check whether it should stop.
//
func (rf *Raft) Kill() {
	atomic.StoreInt32(&rf.dead, 1)
	// Your code here, if desired.
	fmt.Printf("%v is killed\n", rf.me)
}

func (rf *Raft) killed() bool {
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
}

//
// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
//
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

	rf.votedFor = -1

	// Your initialization code here (2A, 2B, 2C).

	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	go rf.bkgRunningCheckVote()
	go rf.bkgRunningAppendEntries()

	return rf
}
