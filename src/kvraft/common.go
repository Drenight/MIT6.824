package kvraft

const (
	OK              = "OK"
	ErrNoKey        = "ErrNoKey"
	ErrWrongLeader  = "ErrWrongLeader"
	ErrLeaderChange = "ErrLeaderChange"
	ErrTimeOut      = "ErrTimeOut"
	ErrDuplicate    = "ErrDuplicate"
)

type Err string

// Put or Append
type ClientRequestID struct {
	ClientID  int
	RequestID int
}

type PutAppendArgs struct {
	ClientRequestID ClientRequestID
	Key             string
	Value           string
	Op              string // "Put" or "Append"
	// You'll have to add definitions here.
	// Field names must start with capital letters,
	// otherwise RPC will break.
}

type PutAppendReply struct {
	Err Err
}

type GetArgs struct {
	ClientRequestID ClientRequestID
	Key             string
	// You'll have to add definitions here.
}

type GetReply struct {
	Err   Err
	Value string
}
