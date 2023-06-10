package kvraft

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"../labgob"
	"../labrpc"
	"../raft"
)

const Debug = 0
const TimeoutInterval = 500 * time.Millisecond

func DPrintf(format string, a ...interface{}) (n int, err error) {
	if Debug > 0 {
		log.Printf(format, a...)
	}
	return
}

type Op struct {
	// Your definitions here.
	// Field names must start with capital letters,
	// otherwise RPC will break.
	Op    string
	Key   string
	Value string
}

type KVServer struct {
	mu      sync.Mutex
	me      int
	rf      *raft.Raft
	applyCh chan raft.ApplyMsg
	dead    int32 // set by Kill()

	maxraftstate int // snapshot if log grows this big

	// Your definitions here.
	mp        map[string]string
	index2req map[int]chan string
}

func (kv *KVServer) Get(args *GetArgs, reply *GetReply) {
	_, isleader := kv.rf.GetState()
	if !isleader {
		reply.Err = ErrWrongLeader
		return
	}
	// Your code here.
	newOp := Op{
		"Get",
		args.Key,
		"",
	}
	index, _, isLeader := kv.rf.Start(newOp)
	if !isLeader {
		reply.Err = ErrWrongLeader
		return
	}
	kv.mu.Lock()
	defer kv.mu.Unlock()
	kv.index2req[index] = make(chan string)
	kv.mu.Unlock()
	select {
	case res := <-kv.index2req[index]:
		reply.Err = OK
		reply.Value = res
	case <-time.After(TimeoutInterval):
		reply.Err = ErrTimeOut
	}
	kv.mu.Lock()
	close(kv.index2req[index])
	delete(kv.index2req, index)
}

func (kv *KVServer) PutAppend(args *PutAppendArgs, reply *PutAppendReply) {
	_, isleader := kv.rf.GetState()
	if !isleader {
		reply.Err = ErrWrongLeader
		return
	}
	// Your code here.
	newOp := Op{}
	if args.Op == "Put" {
		newOp = Op{
			"Put",
			args.Key,
			args.Value,
		}
	} else {
		newOp = Op{
			"Append",
			args.Key,
			args.Value,
		}
	}
	// fmt.Printf("PutAppend, newOP is %+v\n", newOp)
	index, _, _ := kv.rf.Start(newOp)
	// fmt.Printf("PutAppend, index: %+v, isLeader: %+v, newOp: %+v\n", index, isLeader, newOp)

	kv.mu.Lock()
	defer kv.mu.Unlock()
	kv.index2req[index] = make(chan string)
	kv.mu.Unlock()
	select {
	case <-kv.index2req[index]:
		reply.Err = OK
	case <-time.After(TimeoutInterval):
		reply.Err = ErrTimeOut
	}
	kv.mu.Lock()
	close(kv.index2req[index])
	delete(kv.index2req, index)
	// fmt.Printf("leader succ exec on index %+v\n", index)
}

func (kv *KVServer) bkgExecApply() {
	for {
		msg := <-kv.applyCh
		// fmt.Printf("I am %+v, 读掉了啊 %+v\n", kv.me, msg)
		if msg.CommandValid {
			// fmt.Printf("Apply!! %+v\n", msg)
			op := msg.Command.(Op)
			if op.Op == "Get" {
				kv.mu.Lock()
				val := kv.mp[op.Key]
				kv.mu.Unlock()
				ch, ok := kv.index2req[msg.CommandIndex]
				if ok {
					ch <- val
				}
				// kv.index2req[msg.CommandIndex] <- val
			} else if op.Op == "Append" {
				kv.mu.Lock()
				kv.mp[op.Key] += op.Value
				kv.mu.Unlock()
				ch, ok := kv.index2req[msg.CommandIndex]
				if ok {
					ch <- ""
				}
				// kv.index2req[msg.CommandIndex] <- ""
			} else if op.Op == "Put" {
				kv.mu.Lock()
				kv.mp[op.Key] = op.Value
				kv.mu.Unlock()
				ch, ok := kv.index2req[msg.CommandIndex]
				if ok {
					ch <- ""
				}
			} else {
				fmt.Printf("?????\n")
			}
		}
	}
}

// the tester calls Kill() when a KVServer instance won't
// be needed again. for your convenience, we supply
// code to set rf.dead (without needing a lock),
// and a killed() method to test rf.dead in
// long-running loops. you can also add your own
// code to Kill(). you're not required to do anything
// about this, but it may be convenient (for example)
// to suppress debug output from a Kill()ed instance.
func (kv *KVServer) Kill() {
	atomic.StoreInt32(&kv.dead, 1)
	kv.rf.Kill()
	// Your code here, if desired.
}

func (kv *KVServer) killed() bool {
	z := atomic.LoadInt32(&kv.dead)
	return z == 1
}

// servers[] contains the ports of the set of
// servers that will cooperate via Raft to
// form the fault-tolerant key/value service.
// me is the index of the current server in servers[].
// the k/v server should store snapshots through the underlying Raft
// implementation, which should call persister.SaveStateAndSnapshot() to
// atomically save the Raft state along with the snapshot.
// the k/v server should snapshot when Raft's saved state exceeds maxraftstate bytes,
// in order to allow Raft to garbage-collect its log. if maxraftstate is -1,
// you don't need to snapshot.
// StartKVServer() must return quickly, so it should start goroutines
// for any long-running work.
func StartKVServer(servers []*labrpc.ClientEnd, me int, persister *raft.Persister, maxraftstate int) *KVServer {
	// call labgob.Register on structures you want
	// Go's RPC library to marshall/unmarshall.
	labgob.Register(Op{})

	kv := new(KVServer)
	kv.me = me
	kv.maxraftstate = maxraftstate

	// You may need initialization code here.

	kv.applyCh = make(chan raft.ApplyMsg)
	kv.rf = raft.Make(servers, me, persister, kv.applyCh)
	kv.index2req = make(map[int]chan string)
	kv.mp = make(map[string]string)

	// You may need initialization code here.

	go kv.bkgExecApply()

	return kv
}
