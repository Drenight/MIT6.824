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
const TimeoutInterval = 1500 * time.Millisecond

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
	Op              string
	Key             string
	Value           string
	ClientRequestID ClientRequestID
	Err             Err
}

type KVServer struct {
	mu      sync.Mutex
	me      int
	rf      *raft.Raft
	applyCh chan raft.ApplyMsg
	dead    int32 // set by Kill()

	maxraftstate int // snapshot if log grows this big

	// Your definitions here.
	mp                     map[string]string
	index2req              map[int]chan Op
	cli2lastApplyRequestID map[int]Op
}

func (kv *KVServer) Get(args *GetArgs, reply *GetReply) {
	DPrintf("Receive Get, srv %+v, arg %+v\n", kv.me, args)
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
		ClientRequestID{
			args.ClientRequestID.ClientID,
			args.ClientRequestID.RequestID,
		},
		"None",
	}
	index, _, isLeader := kv.rf.Start(newOp)
	if !isLeader {
		DPrintf("Sus GET\n")
		reply.Err = ErrWrongLeader
		return
	}
	kv.mu.Lock()
	defer kv.mu.Unlock()
	kv.index2req[index] = make(chan Op)
	ch := kv.index2req[index]
	kv.mu.Unlock()
	select {
	case op := <-ch:
		if op.Err != "None" {
			// Error during apply
			DPrintf("op'err is %+v\n", op.Err)
			reply.Err = op.Err
			reply.Value = op.Value
			// fmt.Printf("reply value %+v\n", reply.Value)
		} else if op.ClientRequestID.ClientID != args.ClientRequestID.ClientID {
			// Apply successful, but not issued by this request, collision due to same index
			reply.Err = ErrLeaderChange
		} else {
			reply.Err = OK
			reply.Value = op.Value
		}
	case <-time.After(TimeoutInterval):
		reply.Err = ErrTimeOut
	}
	kv.mu.Lock()

	close(kv.index2req[index])
	delete(kv.index2req, index)
}

func (kv *KVServer) PutAppend(args *PutAppendArgs, reply *PutAppendReply) {
	DPrintf("Receive PA, srv %+v, arg %+v\n", kv.me, args)
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
			ClientRequestID{
				args.ClientRequestID.ClientID,
				args.ClientRequestID.RequestID,
			},
			"None",
		}
	} else {
		newOp = Op{
			"Append",
			args.Key,
			args.Value,
			ClientRequestID{
				args.ClientRequestID.ClientID,
				args.ClientRequestID.RequestID,
			},
			"None",
		}
	}
	// fmt.Printf("PutAppend, newOP is %+v\n", newOp)
	index, _, isLeader := kv.rf.Start(newOp)
	if !isLeader {
		DPrintf("Sus PA\n")
		reply.Err = ErrWrongLeader
		return
	}
	// fmt.Printf("PutAppend, index: %+v, isLeader: %+v, newOp: %+v\n", index, isLeader, newOp)

	kv.mu.Lock()
	defer kv.mu.Unlock()
	kv.index2req[index] = make(chan Op)
	ch := kv.index2req[index]
	kv.mu.Unlock()
	select {
	case op := <-ch:
		if op.Err != "None" {
			// Error during apply
			reply.Err = op.Err
		} else if op.ClientRequestID.ClientID != args.ClientRequestID.ClientID {
			// Apply successful, but not issued by this request, collision due to same index
			reply.Err = ErrLeaderChange
		} else {
			reply.Err = OK
		}
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

			kv.mu.Lock()

			//Duplicate check, with the same client's last applied
			last_op, notNew := kv.cli2lastApplyRequestID[op.ClientRequestID.ClientID]
			if notNew && last_op.ClientRequestID.RequestID == op.ClientRequestID.RequestID {
				op = last_op //copy get result
				op.Err = ErrDuplicate
				ch, ok := kv.index2req[msg.CommandIndex]
				DPrintf("Detect Duplicate, srv %+v, client %+v, req %+v\n", kv.me, op.ClientRequestID.ClientID, op.ClientRequestID.RequestID)
				kv.mu.Unlock()
				if ok {
					ch <- op
				}
				continue
			} else {
				DPrintf("Start apply, srv %+v notNew %+v, last_op %+v, op %+v\n", kv.me, notNew, last_op, op)
			}

			if op.Op == "Get" {
				val := kv.mp[op.Key]
				op.Value = val
				ch, ok := kv.index2req[msg.CommandIndex]
				kv.cli2lastApplyRequestID[op.ClientRequestID.ClientID] = op
				if ok {
					kv.mu.Unlock()
					ch <- op
				} else {
					kv.mu.Unlock()
					// DPrintf("Closed Channel, maybe due to timeout\n")
				}
				// kv.index2req[msg.CommandIndex] <- val
			} else if op.Op == "Append" {
				kv.mp[op.Key] += op.Value
				ch, ok := kv.index2req[msg.CommandIndex]
				kv.cli2lastApplyRequestID[op.ClientRequestID.ClientID] = op
				if ok {
					kv.mu.Unlock()
					ch <- op
				} else {
					kv.mu.Unlock()
					// DPrintf("Closed Channel, maybe due to timeout\n")
				}
				// kv.index2req[msg.CommandIndex] <- ""
			} else if op.Op == "Put" {
				kv.mp[op.Key] = op.Value
				ch, ok := kv.index2req[msg.CommandIndex]
				kv.cli2lastApplyRequestID[op.ClientRequestID.ClientID] = op
				if ok {
					kv.mu.Unlock()
					ch <- op
				} else {
					kv.mu.Unlock()
					// DPrintf("Closed Channel, maybe due to timeout\n")
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
	kv.index2req = make(map[int]chan Op)
	kv.mp = make(map[string]string)
	kv.cli2lastApplyRequestID = make(map[int]Op)

	// You may need initialization code here.

	go kv.bkgExecApply()

	return kv
}
