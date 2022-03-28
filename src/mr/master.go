package mr

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"sync"
	"time"
)

type fileStatus int

const fileFree = fileStatus(0)
const fileDone = fileStatus(1)

type Master struct {
	// Your definitions here.
	Files                 []string
	NReduce               int
	FilesStatus           map[int]fileStatus //concurrency
	FileStatusMutex       sync.Mutex
	ReduceFileStatus      map[int]fileStatus
	ReduceFileStatusMutex sync.Mutex
}

// Your code here -- RPC handlers for the worker to call.

//
// an example RPC handler.
//
// the RPC argument and reply types are defined in rpc.go.
//
func (m *Master) Example(args *ExampleArgs, reply *ExampleReply) error {
	reply.Y = args.X + 1
	return nil
}

func (m *Master) ReportMap(args *CallReportMapArgs, reply *interface{}) error {
	num := args.FileNum
	m.FileStatusMutex.Lock()
	if m.FilesStatus[num] != fileDone && m.FilesStatus[num] != fileFree {
		m.FilesStatus[num] = fileDone
	}
	m.FileStatusMutex.Unlock()
	return nil
}
func (m *Master) ReportReduce(args *CallReportReduceArgs, reply *interface{}) error {
	num := args.FileNum
	m.FileStatusMutex.Lock()
	if m.ReduceFileStatus[num] != fileDone && m.ReduceFileStatus[num] != fileFree {
		m.ReduceFileStatus[num] = fileDone
	}
	m.FileStatusMutex.Unlock()
	return nil
}

func (m *Master) AssignReduceTask(args interface{}, reply *AssignReduceTaskReply) error {
	needWork := -1
	allDone := 1
	//var mutex sync.Mutex
	m.FileStatusMutex.Lock()
	for i := 0; i < m.NReduce; i++ {
		if m.ReduceFileStatus[i] != fileDone {
			allDone = 0
		}
		if m.ReduceFileStatus[i] != fileDone && m.ReduceFileStatus[i] != fileFree {
			if int(time.Now().Unix())-int(m.ReduceFileStatus[i]) > 10 {
				m.ReduceFileStatus[i] = fileFree
			}
		}
		if m.ReduceFileStatus[i] == fileFree {
			m.ReduceFileStatus[i] = fileStatus(time.Now().Unix())
			needWork = i
			break
		}
	}
	reply.NReduce = m.NReduce
	reply.FileTotalNum = len(m.Files)

	if allDone == 1 {
		reply.FileNum = -2
	} else {
		reply.FileNum = needWork
	}
	if needWork == -1 {
		reply.FileStatus = fileDone //or none can use
	} else {
		reply.FileStatus = m.ReduceFileStatus[needWork]
	}
	m.FileStatusMutex.Unlock()

	return nil
}

func (m *Master) AssignMapTask(args interface{}, reply *AssignMapTaskReply) error {
	needWork := -1
	allDone := 1
	m.FileStatusMutex.Lock()
	for i := 0; i < len(m.FilesStatus); i++ {
		if m.FilesStatus[i] != fileDone {
			allDone = 0
		}
		//release status to free for too long working -- 10s
		if m.FilesStatus[i] != fileDone && m.FilesStatus[i] != fileFree {
			if int(time.Now().Unix())-int(m.FilesStatus[i]) > 10 {
				m.FilesStatus[i] = fileFree
			}
		}
		if m.FilesStatus[i] == fileFree {
			m.FilesStatus[i] = fileStatus(time.Now().Unix())
			needWork = i
			break
		}
	}

	reply.NReduce = m.NReduce
	reply.FileTotalNum = len(m.Files)
	if allDone == 1 {
		reply.FileNum = -2
		reply.FileName = ""
		reply.FileStatus = fileDone
	} else {
		reply.FileNum = needWork
		if needWork == -1 {
			reply.FileName = ""
			reply.FileStatus = fileDone
		} else {
			reply.FileName = m.Files[needWork]
			reply.FileStatus = m.FilesStatus[needWork]
		}
	}
	m.FileStatusMutex.Unlock()
	return nil
}

//
// start a thread that listens for RPCs from worker.go
//
func (m *Master) server() {
	rpc.Register(m)
	rpc.HandleHTTP()
	//l, e := net.Listen("tcp", ":1234")
	sockname := masterSock()
	os.Remove(sockname)
	l, e := net.Listen("unix", sockname)
	if e != nil {
		log.Fatal("listen error:", e)
	}
	go http.Serve(l, nil)
}

//
// main/mrmaster.go calls Done() periodically to find out
// if the entire job has finished.
//
func (m *Master) Done() bool {
	// Your code here.
	time.Sleep(time.Second * 10)
	fmt.Print("let me see...\n")
	m.ReduceFileStatusMutex.Lock()
	var done = true
	for _, i := range m.ReduceFileStatus {
		if i != fileDone {
			done = false
		}
	}
	m.ReduceFileStatusMutex.Unlock()
	return done
}

//
// create a Master.
// main/mrmaster.go calls this function.
// nReduce is the number of reduce tasks to use.
//
func MakeMaster(files []string, nReduce int) *Master {
	m := Master{
		Files:            files,
		NReduce:          nReduce,
		FilesStatus:      map[int]fileStatus{},
		ReduceFileStatus: map[int]fileStatus{},
	}
	for i := 0; i < len(files); i++ {
		m.FilesStatus[i] = fileFree
	}
	for i := 0; i < nReduce; i++ {
		m.ReduceFileStatus[i] = fileFree
	}
	// Your code here.

	m.server()
	return &m
}
