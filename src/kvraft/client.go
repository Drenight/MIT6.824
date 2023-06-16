package kvraft

import (
	"crypto/rand"
	"math/big"
	"time"

	"../labrpc"
)

type Clerk struct {
	servers  []*labrpc.ClientEnd
	ClientID int
	// You will have to modify this struct.
}

func nrand() int64 {
	max := big.NewInt(int64(1) << 62)
	bigx, _ := rand.Int(rand.Reader, max)
	x := bigx.Int64()
	return x
}

func MakeClerk(servers []*labrpc.ClientEnd) *Clerk {
	ck := new(Clerk)
	ck.servers = servers
	ck.ClientID = int(nrand())
	// You'll have to add code here.
	return ck
}

// fetch the current value for a key.
// returns "" if the key does not exist.
// keeps trying forever in the face of all other errors.
//
// you can send an RPC with code like this:
// ok := ck.servers[i].Call("KVServer.Get", &args, &reply)
//
// the types of args and reply (including whether they are pointers)
// must match the declared types of the RPC handler function's
// arguments. and reply must be passed as a pointer.
func (ck *Clerk) Get(key string) string {
	// fmt.Println("www1")
	// You will have to modify this function.
	args := GetArgs{
		ClientRequestID{
			ck.ClientID,
			int(nrand()),
		},
		key,
	}
	reply := GetReply{}
	// ok := false

	for {
		for index, srv := range ck.servers {
			_ = srv.Call("KVServer.Get", &args, &reply)
			if reply.Err == OK || reply.Err == ErrDuplicate {
				return reply.Value
			} else {
				DPrintf("%+v, len of srv %+v, srv %+v client %+v req %+v reply %+v\n", "Get", len(ck.servers), index, ck.ClientID, args.ClientRequestID.RequestID, reply)
			}
			time.Sleep(time.Millisecond * 10)
		}
	}

}

// shared by Put and Append.
//
// you can send an RPC with code like this:
// ok := ck.servers[i].Call("KVServer.PutAppend", &args, &reply)
//
// the types of args and reply (including whether they are pointers)
// must match the declared types of the RPC handler function's
// arguments. and reply must be passed as a pointer.
func (ck *Clerk) PutAppend(key string, value string, op string) {
	// fmt.Printf("没错吧 key:%+v value:%+v op:%+v\n", key, value, op)
	// You will have to modify this function.
	args := PutAppendArgs{
		ClientRequestID{
			ck.ClientID,
			int(nrand()),
		},
		key,
		value,
		op,
	}
	reply := PutAppendReply{}
	// ok := false

	// fmt.Printf("xxdd %+v\n", len(ck.servers))

	for {
		for index, srv := range ck.servers {
			_ = srv.Call("KVServer.PutAppend", &args, &reply)
			if reply.Err == OK || reply.Err == ErrDuplicate {
				return
			} else {
				DPrintf("%+v, len of srv %+v, srv %+v client %+v req %+v reply %+v\n", op, len(ck.servers), index, ck.ClientID, args.ClientRequestID.RequestID, reply)
			}
			time.Sleep(time.Millisecond * 10)
		}
	}
}

func (ck *Clerk) Put(key string, value string) {
	ck.PutAppend(key, value, "Put")
}
func (ck *Clerk) Append(key string, value string) {
	ck.PutAppend(key, value, "Append")
}
