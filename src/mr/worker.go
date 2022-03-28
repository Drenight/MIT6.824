package mr

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"log"
	"net/rpc"
	"os"
	"plugin"
	"sort"
	"strconv"
	"time"
)

//
// Map functions return a slice of KeyValue.
//
type KeyValue struct {
	Key   string
	Value string
}

// for sorting by key.
type ByKey []KeyValue

// for sorting by key.
func (a ByKey) Len() int           { return len(a) }
func (a ByKey) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByKey) Less(i, j int) bool { return a[i].Key < a[j].Key }

// 一个打乱词序的过程？对所有词进行分组，再开个fileName那样的带锁map去调reduce
// use ihash(key) % NReduce to choose the reduce
// task number for each KeyValue emitted by Map.
//
func ihash(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() & 0x7fffffff)
}

//
// main/mrworker.go calls this function.
//
func Worker(mapf func(string, string) []KeyValue,
	reducef func(string, []string) string) {

	// Your worker implementation here.

	// uncomment to send the Example RPC to the master.
	//CallExample()
	for {
		time.Sleep(time.Second)
		mapDone := callAssignMapTask()
		if mapDone {
			break
		}
	}

	for {
		time.Sleep(time.Second)
		reduceDone := callAssignReduceTask()
		if reduceDone {
			break
		}
	}
}

//
// example function to show how to make an RPC call to the master.
//
// the RPC argument and reply types are defined in rpc.go.
//
func CallExample() {

	// declare an argument structure.
	args := ExampleArgs{}

	// fill in the argument(s).
	args.X = 99

	// declare a reply structure.
	reply := ExampleReply{}

	// send the RPC request, wait for the reply.
	call("Master.Example", &args, &reply)

	// reply.Y should be 100.
	fmt.Printf("reply.Y %v\n", reply.Y)
}

type tmpFile struct {
	filePtr    *os.File
	originName string
	tmpName    string
}

func callAssignReduceTask() bool {
	var args interface{}
	reply := AssignReduceTaskReply{}
	rpcSuccess := call("Master.AssignReduceTask", &args, &reply)
	if !rpcSuccess {
		return false
	}

	if reply.FileNum == -2 {
		return true
	}
	if reply.FileNum == -1 {
		return false
	}

	fmt.Printf("%+v\n", reply)
	_, reducef := loadPlugin(os.Args[1])
	oname := "mr-out-" + strconv.Itoa(reply.FileNum)
	ofile, _ := ioutil.TempFile(".", oname)

	for i := 0; i < reply.FileTotalNum; i++ {
		fileName := "mr-" + strconv.Itoa(i) + "-" + strconv.Itoa(reply.FileNum)
		file, err := os.Open(fileName)
		if err != nil {
			log.Fatalf("cannot open %v", fileName)
			return false
		}
		kva := []KeyValue{}
		dec := json.NewDecoder(file)
		for {
			var kv KeyValue
			if err := dec.Decode(&kv); err != nil {
				break
			}
			kva = append(kva, kv)
		}
		for i < len(kva) {
			j := i + 1
			for j < len(kva) && kva[j].Key == kva[i].Key {
				j++
			}
			values := []string{}
			for k := i; k < j; k++ {
				values = append(values, kva[k].Value)
			}
			output := reducef(kva[i].Key, values)

			// this is the correct format for each line of Reduce output.
			//fmt.Fprintf(ofile, "%v %v\n", intermediate[i].Key, output)
			ofile.WriteString(kva[i].Key + " " + output + "\n")

			i = j
		}
		file.Close()
	}

	nowTS := time.Now().Unix()
	if int(nowTS)-int(reply.FileStatus) > 8 { //for safe, give up this process
		ofile.Close()
		return false
	}

	os.Rename(ofile.Name(), oname)
	flag := callReportReduce(reply.FileNum)
	if !flag {
		log.Fatalf("fail to report reduce")
	}

	ofile.Close()

	return false
}

func callReportReduce(fileNum int) bool {
	args := CallReportReduceArgs{FileNum: fileNum}
	var reply interface{}
	rpcSuccess := call("Master.ReportReduce", &args, &reply)
	return rpcSuccess
}

func callReportMap(fileNum int) bool {
	args := CallReportMapArgs{FileNum: fileNum}
	var reply interface{}
	rpcSuccess := call("Master.ReportMap", &args, &reply)
	return rpcSuccess
}

func callAssignMapTask() bool {
	var args interface{}
	reply := AssignMapTaskReply{}
	rpcSuccess := call("Master.AssignMapTask", &args, &reply)
	if !rpcSuccess {
		return false
	}

	fmt.Printf("reply:%+v\n", reply)

	if reply.FileNum == -2 {
		return true
	}

	if reply.FileNum == -1 {
		//no available
		return false
	}

	// after finish work remember to check timeStamp , exceed 10 mins shall not be submitted
	mapf, _ := loadPlugin(os.Args[1])
	intermediate := []KeyValue{}

	filename := reply.FileName
	file, err := os.Open(filename)
	if err != nil {
		log.Fatalf("cannot open %v", filename)
		return false
	}
	content, err := ioutil.ReadAll(file)
	if err != nil {
		log.Fatalf("cannot read %v", filename)
		return false
	}
	file.Close()
	kva := mapf(filename, string(content))
	intermediate = append(intermediate, kva...)

	sort.Sort(ByKey(intermediate))

	var intermediateFileSlice []tmpFile
	for i := 0; i < reply.NReduce; i++ {
		oname := "mr-" + strconv.Itoa(reply.FileNum) + "-" + strconv.Itoa(i)
		ofile, _ := ioutil.TempFile(".", oname) //os.Create(oname)
		tmp := tmpFile{
			filePtr:    ofile,
			originName: oname,
			tmpName:    ofile.Name(),
		}
		intermediateFileSlice = append(intermediateFileSlice, tmp)
	}

	for i := range intermediate {
		k, _ := intermediate[i].Key, intermediate[i].Value
		hashResult := ihash(k) % reply.NReduce
		//write into file "mr-FileNum-hashResult"
		enc := json.NewEncoder(intermediateFileSlice[hashResult].filePtr)
		err := enc.Encode(&intermediate[i])
		if err != nil {
			log.Fatalf("can not write json")
			return false
		}
	}

	//successfully run here, which means I need to check time whether to report
	nowTS := time.Now().Unix()
	if int(nowTS)-int(reply.FileStatus) > 8 { //for safe, give up this process
		for i := range intermediateFileSlice {
			intermediateFileSlice[i].filePtr.Close()
		}
		return false
	}

	//now this is a successful work

	for i := range intermediateFileSlice {
		err := os.Rename(intermediateFileSlice[i].tmpName, intermediateFileSlice[i].originName)
		if err != nil {
			log.Fatalf("fail to rename file")
			return false
		}
		intermediateFileSlice[i].filePtr.Close()
	}

	flag := callReportMap(reply.FileNum)
	if !flag {
		log.Fatalf("fail to report map")
	}

	return false
}

//
// send an RPC request to the master, wait for the response.
// usually returns true.
// returns false if something goes wrong.
//
func call(rpcname string, args interface{}, reply interface{}) bool {
	// c, err := rpc.DialHTTP("tcp", "127.0.0.1"+":1234")
	sockname := masterSock()
	c, err := rpc.DialHTTP("unix", sockname)
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer c.Close()

	err = c.Call(rpcname, args, reply)
	if err == nil {
		return true
	}

	fmt.Println(err)
	return false
}

func loadPlugin(filename string) (func(string, string) []KeyValue, func(string, []string) string) {
	p, err := plugin.Open(filename)
	if err != nil {
		log.Fatalf("cannot load plugin %v", filename)
	}
	xmapf, err := p.Lookup("Map")
	if err != nil {
		log.Fatalf("cannot find Map in %v", filename)
	}
	mapf := xmapf.(func(string, string) []KeyValue)
	xreducef, err := p.Lookup("Reduce")
	if err != nil {
		log.Fatalf("cannot find Reduce in %v", filename)
	}
	reducef := xreducef.(func(string, []string) string)

	return mapf, reducef
}
