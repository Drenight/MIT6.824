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
	Cmd   interface{}
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

	isLeader      bool
	logs          []LogEntry
	commitIndex   int64
	lastApplied   int64
	nextIndexMap  map[int]int64
	matchIndexMap map[int]int64
}

func (rf *Raft) beLeader() {
	rf.isLeader = true
	for i := 0; i < len(rf.peers); i++ {
		rf.nextIndexMap[i] = 0
		rf.matchIndexMap[i] = 0
	}
	fmt.Printf("**I, %v become leader of Term %v\n", rf.me, rf.term)
}

func (rf *Raft) bkgRunningCheckVote() {
	for { // test for starting leader vote
		if rf.killed() {
			//fmt.Printf("%v this is dead\n", rf.me)
			//time.Sleep(time.Millisecond * time.Duration(5))
			return
		} else { //alive, examine whether receive hb from leader
			//fmt.Printf("try lock %v ", rf.me)
			rf.GetMutex()
			//fmt.Printf("lock %v!", rf.me)
			if rf.isLeader {
				rf.ReleaseMutex()
				//fmt.Printf("release lock %v as leader\n", rf.me)
				time.Sleep(time.Second)
				//fmt.Printf("I, %v, is a leader\n", rf.me)
				continue
			}
			rf.hasLeader = false
			rf.ReleaseMutex()
			//fmt.Printf("release lock %v, reset my hasLeader!\n", rf.me)
			r := rand.New(rand.NewSource(time.Now().UnixNano()))
			time.Sleep(time.Millisecond*time.Duration(voteExpire) + time.Millisecond*time.Duration(r.Int()%99))

			//fmt.Printf("Try %v...", rf.me)
			rf.GetMutex()
			//fmt.Printf("Ok %v!", rf.me)
			if /*rf.isLeader ||*/ rf.hasLeader /*|| (rf.votedFor != -1 && rf.votedFor != rf.me)*/ { //has get new leader OR has voted in this term
				fmt.Printf("I, %v already has leader or voted, voted for is %v ", rf.me, rf.votedFor)
				rf.ReleaseMutex()
				//fmt.Printf("Release lock %v!\n", rf.me)
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
				//fmt.Printf("Release lock %v!\n", rf.me)

				//wg.Add(len(rf.peers) - 1)

				//allCnt := 0
				startTime := time.Now().UnixMilli()
				//startTimeMillie := time.Now().UnixMilli()

				for i := 0; i < len(rf.peers); i++ {
					if i == rf.me {
						continue
					}
					tmp := i //classic bug, using iteration args with multithread
					go func(i int) {
						reply := RequestVoteReply{}
						st := time.Now().UnixMilli()
						rf.sendRequestVote(i, &args, &reply /*, &wg*/)
						rf.GetMutex()
						fmt.Printf("%+v consume%v vote req done, now my votedfor%v get %+v, cntYes%v, half%v\n", rf.me, time.Now().UnixMilli()-st, rf.votedFor, reply, cntYes, len(rf.peers)/2)
						defer rf.ReleaseMutex()
						//if rf.votedFor != rf.me {
						//	return
						//}
						//if reply.Term > rf.term {
						//	rf.term = reply.Term
						//	rf.votedFor = -1
						//	return
						//}
						if reply.VoteAsLeader {
							cntYes += 1
							//if cntYes == len(rf.peers)/2+1 {
							//	rf.beLeader()
							//	rf.doHeartBeat()
							//}
						} else if !reply.VoteAsLeader {
							cntNo += 1
						}
					}(tmp)
				}

				for {
					time.Sleep(time.Millisecond * time.Duration(5))
					rf.GetMutex()
					if rf.hasLeader {
						rf.ReleaseMutex()
						break
					}
					if cntYes > len(rf.peers)/2 { //odd
						rf.beLeader()
						rf.doHeartBeat()
						rf.votedFor = -1 //?不一定要改，通过高term覆盖就可以了？
						rf.ReleaseMutex()
						break
					}
					if cntNo > len(rf.peers)/2 {
						rf.votedFor = -1
						rf.ReleaseMutex()
						break
					}
					if time.Now().UnixMilli()-startTime > 50 {
						rf.votedFor = -1
						rf.ReleaseMutex()
						break
					}
					rf.ReleaseMutex()
				}

				//fmt.Printf("**I am %v, get %v votes, and %v noVotes, total is %v, sum less means expire!\n", rf.me, cntYes, cntNo, len(rf.peers))
			}
		}
	}
}
func Min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

//under mutex
func (rf *Raft) doHeartBeat() {
	if !rf.isLeader {
		return
	}
	for i := 0; i < len(rf.peers); i++ {
		if i == rf.me {
			continue
		}
		args := AppendEntriesArgs{IsHeartBeat: true, LeaderID: int64(rf.me), LeaderTerm: rf.term}
		reply := AppendEntriesReply{}
		go func(to int, args *AppendEntriesArgs, reply *AppendEntriesReply) {
			rf.sendAppendEntries(to, args, reply)
		}(i, &args, &reply)
	}
}

func (rf *Raft) bkgRunningAppendEntries() {
	for {
		if rf.killed() {
			//fmt.Printf("%v this is dead\n", rf.me)
			//time.Sleep(time.Millisecond * time.Duration(5))
			return
		} else {
			time.Sleep(time.Millisecond * time.Duration(100))
			ts := time.Now().UnixNano()
			//fmt.Printf("$$I, %v, is leader of term%v, Try mutex for my AE!\n", rf.me, rf.term)
			rf.GetMutex()
			if rf.isLeader {
				fmt.Printf("$$I, %v, is leader of term%v, mutex get for my AE!, consumes %v\n", rf.me, rf.term, time.Now().UnixNano()-ts)
				rf.doHeartBeat()
				rf.ReleaseMutex()
				//fmt.Printf("$$I, %v, is leader of term%v, start my AE!\n", rf.me, rf.term)

				//var wg sync.WaitGroup
				//wg.Add(len(rf.peers) - 1)
				//cntDone := 0
				//commitIndexNow := -1

				//rf.GetMutex()
				//fmt.Printf("$$I, %v, is leader of term%v, Mutex for AE args get!\n", rf.me, rf.term)

				/*
					var argsList []AppendEntriesArgs
					var replyList []AppendEntriesReply

					for i := 0; i < len(rf.peers); i++ {
						if i == rf.me {
							argsList = append(argsList, AppendEntriesArgs{})
							replyList = append(replyList, AppendEntriesReply{})
							continue
						}

							var entries []LogEntry

							//for j := rf.nextIndexMap[i]; j < int64(len(rf.logs)); j++ {
							//	entries = append(entries, rf.logs[j])
							//}
							entries = append(entries, rf.logs[rf.nextIndexMap[i]:]...)
							//fmt.Printf("%+v\n", entries)
							prevLogIndex := int64(-1)
							prevLogTerm := int64(-1)
							if len(rf.logs) > 1 {
								prevLogIndex = rf.logs[len(rf.logs)-2].Index
								prevLogTerm = rf.logs[len(rf.logs)-2].Term
							}

							args := AppendEntriesArgs{
								LeaderID:   int64(rf.me),
								LeaderTerm: rf.term,
								//PrevLogTerm:	//有prev，清掉follower后面；没prev，减leader的map；记得是无限回退找lca的
								PrevLogIndex: prevLogIndex,
								PrevLogTerm:  prevLogTerm,
								Entries:      entries,
								LeaderCommit: rf.commitIndex,
							}


						//测试args
						args := AppendEntriesArgs{LeaderID: int64(rf.me), LeaderTerm: rf.term}
						reply := AppendEntriesReply{}
						argsList = append(argsList, args)
						replyList = append(replyList, reply)
					}
				*/
				//rf.ReleaseMutex()

				//fmt.Printf("$$I, %v, is leader of term%v, AE args ready!\n", rf.me, rf.term)

				/*
					for i := 0; i < len(rf.peers); i++ {
						if i == rf.me {
							continue
						}
				*/
				/*
					if i == rf.me {
						continue
					}

					rf.GetMutex()
					var entries []LogEntry

						for j := rf.nextIndexMap[i]; j < int64(len(rf.logs)); j++ {
							entries = append(entries, rf.logs[j])
						}

					//fmt.Printf("%+v\n", entries)
					prevLogIndex := int64(-1)
					prevLogTerm := int64(-1)
					if len(rf.logs) > 1 {
						prevLogIndex = rf.logs[len(rf.logs)-2].Index
						prevLogTerm = rf.logs[len(rf.logs)-2].Term
					}

					args := AppendEntriesArgs{
						LeaderID:   int64(rf.me),
						LeaderTerm: rf.term,
						//PrevLogTerm:	//有prev，清掉follower后面；没prev，减leader的map；记得是无限回退找lca的
						PrevLogIndex: prevLogIndex,
						PrevLogTerm:  prevLogTerm,
						Entries:      entries,
						LeaderCommit: rf.commitIndex,
					}
					reply := AppendEntriesReply{}

					rf.ReleaseMutex()
				*/
				//go rf.sendAppendEntries(i, &args, &reply)
				/*
					tmp := i
					go func(i int, args AppendEntriesArgs, reply AppendEntriesReply) { //while true here
						rf.sendAppendEntries(i, &args, &reply)
						//fmt.Printf("%+v\n", args)
						//fmt.Printf("%+v\n", len(args.Entries))
				*/
				/*
					if reply.Success == 1 {
						rf.GetMutex()
						if commitIndexNow == -1 {
							if len(args.Entries) != 0 {
								fmt.Printf("www\n")
								commitIndexNow = int(args.Entries[0].Index)
							}
						} else {
							commitIndexNow = Min(commitIndexNow, int(args.Entries[0].Index))
						}
						if len(args.Entries) != 0 {
							rf.matchIndexMap[i] = args.Entries[len(args.Entries)-1].Index
							rf.nextIndexMap[i] = rf.matchIndexMap[i] + 1
						}
						cntDone += 1
						rf.ReleaseMutex()
					} else {
						fmt.Printf("?,I %v got rej from %v\n", rf.me, i)
						rf.GetMutex()
						cntDone += 1
						rf.ReleaseMutex()
					}
				*/
				//}(tmp, AppendEntriesArgs{LeaderID: int64(rf.me), LeaderTerm: rf.term}, AppendEntriesReply{} /*argsList[i], replyList[i]*/)
				//}
				//st := time.Now().UnixMilli()
				//fmt.Printf("I %v, start waiting for AE reply\n", rf.me)

				/*
					for {
						//break

							if time.Now().UnixMilli()-st > 1000 {
								break
							}

						time.Sleep(time.Millisecond * time.Duration(10))
						rf.GetMutex()
						//fmt.Printf("cntDone is %+v\n", cntDone)
						if cntDone == len(rf.peers)-1 { //sub me
							//fmt.Printf("I have commit Index Now %+v\n", commitIndexNow)
							rf.commitIndex = int64(Min(int(rf.commitIndex), commitIndexNow))
							//rf.ReleaseMutex()
							//break
						}
						rf.ReleaseMutex()
					}
				*/

				//fmt.Printf("I %v collect %v AE reply consuming %v\n", rf.me, cntDone, time.Now().UnixMilli()-st)
				//rf.ReleaseMutex()
				//time.Sleep(time.Millisecond * time.Duration(90))
				//wg.Wait()
				//time.Sleep(time.Millisecond * time.Duration(95))
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
	IsHeartBeat  bool
	LeaderID     int64
	LeaderTerm   int64
	PrevLogIndex int64
	PrevLogTerm  int64
	Entries      []LogEntry
	LeaderCommit int64
}

//
// example RequestVote RPC reply structure.
// field names must start with capital letters!
//
type RequestVoteReply struct {
	// Your data here (2A).
	VoteAsLeader bool
	Term         int64
}
type AppendEntriesReply struct {
	FollowerTerm int64
	Success      int64 //follower's last log is prev
}

//
// example RequestVote RPC handler.
//
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (2A, 2B).
	//fmt.Printf("I am called by %v, going to LOCK %v", args.Id, rf.me)
	rf.GetMutex() //BUG DEAD LOCK?
	//fmt.Printf("I am called by %v, LOCKED %v", args.Id, rf.me)

	fmt.Printf("I am %v, args is %+v\n", rf.me, args)

	//if rf.term < args.Term {
	//	rf.term = args.Term
	//	rf.votedFor = -1
	//}

	if rf.term < args.Term && rf.votedFor == -1 || rf.votedFor == args.Id { //todo,split vote?
		rf.votedFor = args.Id
		reply.VoteAsLeader = true
		reply.Term = rf.term
		fmt.Printf("I, %v, vote %v YES\n", rf.me, args.Id)
	} else {
		reply.VoteAsLeader = false
		reply.Term = rf.term
		fmt.Printf("I, %v, vote %v NO\n", rf.me, args.Id)
	}

	rf.ReleaseMutex()
	//fmt.Printf("I am called by %v, going to RELEASE %v\n", args.Id, rf.me)
}
func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.GetMutex()
	if rf.term > args.LeaderTerm {
		fmt.Printf("Pity, I %+v have term%+v, args is %+v\n", rf.me, rf.term, args)
		reply.Success = 0
		reply.FollowerTerm = rf.term
		rf.ReleaseMutex()
		return
	}
	//heartBeat impact
	if rf.isLeader {
		fmt.Printf("!!!!!!!!!!!!!!!!!I %+v, hear from %+v, be follower %+v\n", rf.me, args.LeaderID, rf.logs)
	}
	rf.isLeader = false
	rf.hasLeader = true
	rf.votedFor = -1
	rf.term = args.LeaderTerm
	//Append Entries impact
	//findPrev := false

	//fmt.Printf("I have log %+v, args is %+v\n", rf.logs, args)
	/*
		if len(rf.logs) == 0 || args.PrevLogIndex == -1 && args.PrevLogTerm == -1 {
			if len(rf.logs) == 0 || len(args.Entries) == 0 || rf.logs[len(rf.logs)-1] != args.Entries[len(args.Entries)-1] {
				rf.logs = append(rf.logs, args.Entries...)
			}
			findPrev = true
		}
		if !findPrev {
			for i, log := range rf.logs {
				if log.Index == args.PrevLogIndex && log.Term == args.PrevLogTerm {
					findPrev = true
					rf.logs = rf.logs[:i+1]
					rf.logs = append(rf.logs, args.Entries...)
					break
				}
			}
		}

		if !findPrev {
			reply.Success = 0
		} else {
			reply.Success = 1
		}
		reply.FollowerTerm = rf.term
	*/
	//fmt.Printf("I,%+v, has log:%+v, I receive %+v\n", rf.me, rf.logs, args)

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
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply /*, wg *sync.WaitGroup*/) bool {
	//fmt.Printf("I am calling to server %v, by %v\n", server, args.Id)
	//defer wg.Done()
	//defer fmt.Printf("requestVote's %v is ok!", server)
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply) //TODO, kick off a new routine with expiration?
	//fmt.Printf("I have called to server %v, by %v,ok is %v\n", server, args.Id, reply.VoteAsLeader)
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
func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	//defer wg.Done()
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	//fmt.Printf("$$Leader heartBeat from %v to %v\n", rf.me, server)
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
	rf.GetMutex()
	index = len(rf.logs)
	term = int(rf.term)
	isLeader = rf.isLeader

	if !isLeader {
		rf.ReleaseMutex()
		return index, term, isLeader
	}

	log := LogEntry{
		Cmd:   command,
		Term:  rf.term,
		Index: int64(len(rf.logs)),
	}

	rf.logs = append(rf.logs, log)

	rf.ReleaseMutex()
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
	rf.nextIndexMap = make(map[int]int64)
	rf.matchIndexMap = make(map[int]int64)

	rf.votedFor = -1

	// Your initialization code here (2A, 2B, 2C).

	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	go rf.bkgRunningCheckVote()
	go rf.bkgRunningAppendEntries()

	return rf
}
