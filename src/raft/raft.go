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
	applyCh   chan ApplyMsg
	hasLeader bool
	term      int64 //term ME kept in logs
	votedFor  int   //leader voted for in this term, -1 for NULL, self means is candidate
	isLeader  bool
	logs      []LogEntry //blank occupy index0, so start from 1
	//Volatile state on all servers:
	commitIndex int64 //whole cluster's highest commit
	lastApplied int64 //my last applied log index

	//Volatile state on leaders:
	nextIndexMap  map[int]int64
	matchIndexMap map[int]int64
}

func (rf *Raft) beLeader() {
	rf.isLeader = true
	for i := 0; i < len(rf.peers); i++ {
		//for each server, index of the next log entry to send to that server (initialized to leader last log index + 1)
		rf.nextIndexMap[i] = rf.logs[len(rf.logs)-1].Index + 1
		//for each server, index of highest log entry known to be replicated on server (initialized to 0, increases monotonically)
		rf.matchIndexMap[i] = 0
	}
	fmt.Printf("**I, %v become leader of Term %v\n", rf.me, rf.term)
}

func (rf *Raft) beFollowerStepDownWithoutLeader(term int64) {
	rf.isLeader = false
	rf.votedFor = -1
	rf.term = term
	fmt.Printf("I %v become follower at term %v\n", rf.me, term)
}

func (rf *Raft) applyAfterCommitChange() {
	if rf.commitIndex > rf.lastApplied {
		for now := rf.lastApplied + 1; now <= int64(Min(int(rf.commitIndex), int(rf.logs[len(rf.logs)-1].Index))); now++ { //全集群的commit可能大于本机log长度
			commitMsg := ApplyMsg{
				CommandValid: true,
				Command:      rf.logs[now].Cmd,
				CommandIndex: int(now),
			}
			rf.applyCh <- commitMsg
			rf.lastApplied = int64(commitMsg.CommandIndex)
		}
	}
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
			if rf.hasLeader { //has get new leader OR has voted in this term
				rf.ReleaseMutex()
				continue
			} else { //be candidate, start to sendRequestVote

				//todo
				//If AppendEntries RPC received from new leader: convert to follower
				//If election timeout elapses: start new election

				fmt.Printf("I, %v, think there's no leader \n", rf.me)

				rf.term++
				rf.votedFor = rf.me

				args := RequestVoteArgs{
					Term:         rf.term,
					Id:           rf.me,
					LastLogIndex: rf.logs[len(rf.logs)-1].Index,
					LastLogTerm:  rf.logs[len(rf.logs)-1].Term,
				}
				//replys := make([]RequestVoteReply, len(rf.peers))
				cntYes := 1
				cntNo := 0
				//var wg sync.WaitGroup
				rf.ReleaseMutex()
				startTime := time.Now().UnixMilli()

				faceHighTerm := -1

				for i := 0; i < len(rf.peers); i++ {
					if i == rf.me {
						continue
					}
					tmp := i //classic bug, using iteration args with multithread
					go func(i int) {
						reply := RequestVoteReply{}
						rf.sendRequestVote(i, &args, &reply)
						rf.GetMutex()
						defer rf.ReleaseMutex()
						if reply.VoteGranted {
							cntYes += 1
						} else if !reply.VoteGranted {
							if reply.Term > rf.term {
								faceHighTerm = int(reply.Term)
							}
							cntNo += 1
						}
					}(tmp)
				}

				rf.GetMutex()
				if faceHighTerm != -1 {
					rf.beFollowerStepDownWithoutLeader(int64(faceHighTerm))
					rf.ReleaseMutex()
					continue
				}
				rf.ReleaseMutex()

				for {
					time.Sleep(time.Millisecond * time.Duration(5))
					rf.GetMutex()
					//If AppendEntries RPC received from new leader: convert to follower
					if rf.hasLeader /*|| rf.votedFor != rf.me*/ {
						rf.ReleaseMutex()
						break
					}
					if cntYes > len(rf.peers)/2 { //odd
						rf.beLeader()
						//Upon election: send initial empty AppendEntries RPCs (heartbeat) to each server; repeat during idle periods to prevent election timeouts (§5.2)
						rf.doHeartBeat()
						rf.votedFor = -1
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
func Max(x, y int) int {
	return x ^ y ^ Min(x, y)
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
		args := AppendEntriesArgs{LeaderID: int64(rf.me), LeaderTerm: rf.term}
		reply := AppendEntriesReply{}
		go func(to int, args *AppendEntriesArgs, reply *AppendEntriesReply) {
			rf.sendAppendEntries(to, args, reply)
		}(i, &args, &reply)
	}
}

func (rf *Raft) bkgRunningAppendEntries() {
	for {
		if rf.killed() {
			return
		} else {
			time.Sleep(time.Millisecond * time.Duration(100))
			rf.GetMutex()
			if rf.isLeader {
				//fmt.Printf("$$I, %v, is leader of term%v, mutex get for my AE!, consumes %v\n", rf.me, rf.term, time.Now().UnixNano()-ts)
				//rf.doHeartBeat()
				rf.ReleaseMutex()
				//fmt.Printf("$$I, %v, is leader of term%v, start my AE!\n", rf.me, rf.term)
				//continue
				//var wg sync.WaitGroup
				//wg.Add(len(rf.peers) - 1)
				cntDone := 0
				faceHighTerm := -1
				for i := 0; i < len(rf.peers); i++ {
					if i == rf.me {
						continue
					}

					tmp := i
					//Do replicate to server i
					go func(i int) {
						rf.GetMutex()
						var entries []LogEntry
						prevLogIndex := int64(0)
						prevLogTerm := int64(0)
						//fmt.Printf("I %+v have log%+v, matchMap%+v,nextMap%+v\n", rf.me, rf.logs, rf.matchIndexMap, rf.nextIndexMap)

						//If last log index ≥ nextIndex for a follower: send AppendEntries RPC with log entries starting at nextIndex
						if int(rf.logs[len(rf.logs)-1].Index) >= int(rf.nextIndexMap[i]) {
							prevLogIndex = rf.logs[rf.nextIndexMap[i]-1].Index //也感觉不是rf.matchIndexMap[i] //感觉不是rf.logs[len(rf.logs)-2].Index //本轮失配，只是sub一下nextmap，交给下一轮，不尝试一轮内同步，怕和后一轮冲突
							prevLogTerm = rf.logs[prevLogIndex].Term
							entries = append(entries, rf.logs[rf.nextIndexMap[i]:]...)
						}

						args := AppendEntriesArgs{
							LeaderID:     int64(rf.me),
							LeaderTerm:   rf.term,
							PrevLogIndex: prevLogIndex,
							PrevLogTerm:  prevLogTerm,
							Entries:      entries,
							LeaderCommit: rf.commitIndex,
							//below two didn't show on figure 2, I use them to check up-to-date
							LastLogIndex: rf.logs[len(rf.logs)-1].Index,
							LastLogTerm:  rf.logs[len(rf.logs)-1].Term,
						}
						reply := AppendEntriesReply{}
						fmt.Printf("***[LEADER]I am %+v,to %+v,AE args is %+v, I have log%+v,matchIndexMap%+v,nextIndexMap%+v\n", rf.me, tmp, args, rf.logs, rf.matchIndexMap, rf.nextIndexMap)
						rf.ReleaseMutex()
						rf.sendAppendEntries(i, &args, &reply)

						//fmt.Printf("%+v\n", len(args.Entries))
						if reply.Success == 1 {
							//If successful: update nextIndex and matchIndex for follower (§5.3)
							rf.GetMutex()
							if len(args.Entries) != 0 {
								rf.matchIndexMap[i] = args.Entries[len(args.Entries)-1].Index
								rf.nextIndexMap[i] = rf.matchIndexMap[i] + 1
							}
							cntDone += 1
							rf.ReleaseMutex()
						} else {
							//fmt.Printf("?,I %v got rej from %v,reason %v\n", rf.me, i, reply.Success)
							rf.GetMutex()
							if reply.Term > rf.term {
								faceHighTerm = int(reply.Term)
							}
							cntDone += 1
							//If AppendEntries fails because of log inconsistency: decrement nextIndex and retry (§5.3)
							if reply.Success == -1 {
								rf.nextIndexMap[i] -= 1
							}
							rf.ReleaseMutex()
						}
					}(tmp)
				}
				st := time.Now().UnixMilli()
				//fmt.Printf("I %v, start waiting for AE reply\n", rf.me)

				//waiter...
				for {
					time.Sleep(time.Millisecond * time.Duration(5))
					rf.GetMutex()
					if time.Now().UnixMilli()-st > 50 {
						rf.ReleaseMutex()
						break
					}
					if cntDone == len(rf.peers)-1 { //sub me
						rf.ReleaseMutex()
						break
					}
					rf.ReleaseMutex()
				}

				rf.GetMutex()
				//If RPC request or response contains term T > currentTerm: set currentTerm = T, convert to follower (§5.1)
				if faceHighTerm != -1 {
					rf.beFollowerStepDownWithoutLeader(int64(faceHighTerm))
					rf.ReleaseMutex()
					continue
				}

				//If there exists an N such that N > commitIndex, a majority of matchIndex[i] ≥ N, and log[N].term == currentTerm: set commitIndex = N (§5.3, §5.4).
				maxMatch := -1
				maxMatchTerm := -1
				lb := 0
				rb := len(rf.logs)
				for lb <= rb {
					mid := (lb + rb) / 2
					tstCounter := 1 //自己
					for it, _ := range rf.peers {
						if it == rf.me {
							continue
						}
						if rf.matchIndexMap[it] >= int64(mid) {
							tstCounter += 1
						}
					}
					if tstCounter > len(rf.peers)/2 {
						maxMatch = Max(maxMatch, mid)
						maxMatchTerm = int(rf.logs[mid].Term)
						lb = mid + 1
					} else {
						rb = mid - 1
					}
				}
				//and log[N].term == currentTerm (Low term can not be committed independently)
				if maxMatch > int(rf.commitIndex) && rf.term == int64(maxMatchTerm) {
					rf.commitIndex = int64(maxMatch)
					//If commitIndex > lastApplied: increment lastApplied, apply log[lastApplied] to state machine (§5.3)
					rf.applyAfterCommitChange()
				}

				rf.ReleaseMutex()
				//fmt.Printf("\n_______%+v, has commitIndex %v\n", rf.me, rf.commitIndex)

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
	Id           int
	Term         int64
	LastLogIndex int64
	LastLogTerm  int64
}
type AppendEntriesArgs struct {
	LeaderID     int64
	LeaderTerm   int64
	PrevLogIndex int64
	PrevLogTerm  int64
	Entries      []LogEntry
	LeaderCommit int64
	LastLogIndex int64
	LastLogTerm  int64
}

//
// example RequestVote RPC reply structure.
// field names must start with capital letters!
//
type RequestVoteReply struct {
	// Your data here (2A).
	VoteGranted bool
	Term        int64
}
type AppendEntriesReply struct {
	Term    int64
	Success int64 //follower's last log is prev
}

//
// example RequestVote RPC handler.
//
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (2A, 2B).
	fmt.Printf("RV------I am %v,args is %+v\n", rf.me, args)
	rf.GetMutex() //BUG DEAD LOCK?
	defer rf.ReleaseMutex()

	//Reply false if term < currentTerm (§5.1)
	if args.Term < rf.term {
		reply.VoteGranted = false
		reply.Term = rf.term
		return
	}

	//?
	/*
		if rf.isLeader {
			reply.VoteGranted = false
			reply.Term = rf.term
			return
		}
	*/

	//If votedFor is null or candidateId, and candidate’s log is at least as up-to-date as receiver’s log, grant vote (§5.2, §5.4)
	if args.LastLogTerm < rf.logs[len(rf.logs)-1].Term {
		fmt.Printf("************************************%+v...%+v\n", args.LastLogTerm, rf.logs[len(rf.logs)-1].Term)
		reply.VoteGranted = false
		reply.Term = rf.term
		return
	} else if args.LastLogTerm == rf.logs[len(rf.logs)-1].Term && args.LastLogIndex < rf.logs[len(rf.logs)-1].Index {
		fmt.Printf("************************************%+v...%+v\n", args.LastLogIndex, rf.logs[len(rf.logs)-1].Index)
		reply.VoteGranted = false
		reply.Term = rf.term
		return
	}

	//If RPC request or response contains term T > currentTerm: set currentTerm = T, convert to follower (§5.1)
	if args.Term > rf.term && rf.isLeader {
		rf.beFollowerStepDownWithoutLeader(args.Term)
	}

	if rf.votedFor == -1 || rf.votedFor == args.Id {
		rf.term = args.Term
		rf.votedFor = args.Id
		reply.VoteGranted = true
		reply.Term = rf.term
	} else {
		reply.VoteGranted = false
		reply.Term = rf.term
	}
}

//0 means term, -1 means didn't find
func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.GetMutex()
	defer rf.ReleaseMutex()
	fmt.Printf("AE------I am %v,args is %+v\n", rf.me, args)
	if args.LeaderTerm < rf.term {
		fmt.Printf("Pity, I %+v have term%+v, args is %+v\n", rf.me, rf.term, args)
		reply.Success = 0
		reply.Term = rf.term
		return
	}

	if args.LastLogTerm < rf.logs[len(rf.logs)-1].Term {
		reply.Success = 0
		reply.Term = rf.term
		fmt.Printf("Pity, I %+v have term%+v, args is %+v\n", rf.me, rf.term, args)
		return
	} else if args.LastLogTerm == rf.logs[len(rf.logs)-1].Term && args.LastLogIndex < rf.logs[len(rf.logs)-1].Index {
		reply.Success = 0
		reply.Term = rf.term
		fmt.Printf("Pity, I %+v have term%+v, args is %+v\n", rf.me, rf.term, args)
		return
	}

	//heartBeat impact
	if rf.isLeader {
		fmt.Printf("!!!!!!!!!!!!!!!!!I %+v, hear from %+v, be follower, my log %+v, his log%+v\n", rf.me, args.LeaderID, rf.logs, args.Entries)
		fmt.Printf("XXXXX his lastLogTerm:%+v, my lastLogTerm%+v\n", args.LastLogTerm, rf.logs[len(rf.logs)-1].Term)
		fmt.Printf("YYYYY his lastlogIndex:%+v, my lastLogIndex%+v\n", args.LastLogIndex, rf.logs[len(rf.logs)-1].Index)
		//rf.beFollowerStepDownWithLeader(args.LeaderTerm, args.LeaderID)
		//rf.beFollowerStepDownWithoutLeader(args.LeaderTerm)
	}
	rf.isLeader = false
	rf.hasLeader = true
	rf.votedFor = -1
	rf.term = args.LeaderTerm
	reply.Success = 1
	reply.Term = rf.term

	//Append Entries impact 1: update logs
	findPrev := false

	fmt.Printf("I have log %+v, args is %+v\n", rf.logs, args)

	rf.hasLeader = true

	if len(args.Entries) == 0 { //心跳
		findPrev = true
		rf.logs = append(rf.logs, args.Entries...)
	}
	if !findPrev { //args有log，开始在自己的log找prev
		for i, log := range rf.logs {
			if log.Index == args.PrevLogIndex && log.Term == args.PrevLogTerm { //冲突term的删掉
				findPrev = true
				rf.logs = rf.logs[:i+1]
				rf.logs = append(rf.logs, args.Entries...)
				break
			}
		}
	}

	if !findPrev {
		reply.Success = -1
		reply.Term = rf.term
		return
	}

	//Append Entries impact 2: update my commitIndex to min (new,leader's)
	if args.LeaderCommit > rf.commitIndex {
		nxt := int64(Min(int(rf.logs[len(rf.logs)-1].Index), int(args.LeaderCommit)))
		rf.commitIndex = nxt
		rf.applyAfterCommitChange()
	}
	fmt.Printf("Follower, My commitIndex %v, findPrev%v\n", rf.commitIndex, findPrev)

	//fmt.Printf("I,%+v, has log:%+v, I receive %+v\n", rf.me, rf.logs, args)
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
	if reply.VoteGranted {
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

	fmt.Printf("Receive input log: id%v, now log%+v\n", rf.me, rf.logs)

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
	rf.applyCh = applyCh
	rf.peers = peers
	rf.persister = persister
	rf.me = me
	rf.nextIndexMap = make(map[int]int64)
	rf.matchIndexMap = make(map[int]int64)
	rf.commitIndex = -1

	rf.votedFor = -1
	rf.logs = append(rf.logs, LogEntry{nil, 0, 0})

	// Your initialization code here (2A, 2B, 2C).

	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	go rf.bkgRunningCheckVote()
	go rf.bkgRunningAppendEntries()

	return rf
}
