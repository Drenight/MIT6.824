# ABSTRACT
enterprise-grade fault-tolerant virtual machines
- replicating the execution of primary VM via backup VM on another server

# 1. Introduction
common: backup server always available to take over if the primary fails
- hidden to external clients, no data lost
- way: ship changes of all state to the backup server
  - bandwidth needed to send this state, particular changes in memory, can be very large

replicate servers using less memory: state-machine approach
- model the servers as deterministic state machines, kept in sync by starting with same initial state and ensuring them receiving the same input reqs
- extra coordination to sync some non-deterministic operations, requiring small network width
- physical servers are difficult to sync since processor frequencies may increase, in contrast to VM

# 2 Basic FT Design
- virtual disks for VMs are on shared storage, accessible for primary and backup for I/O

All input primary VM receives is forwarded via network connection called **logging channel**

backups' output dropped by hypervisor

## 2.1 Deterministic Replay Implementation
solution: VMware deterministic replay
- record inputs of a VM and all possible non-de associated with execution in a stream of log entries written to a log file
  - exactly replayed reading this log
  - for non-de ops, sufficient information is logged, op can be reproduced with the same state change and output

hardware performance counters
- special-purpose registers built into modern microprocessors to store the counts of hardware-related activities within computer systems

epoch
- batch solve with running interrupt last
- no need in FT cause it's efficient enought to run immediately

## 2.2 FT Protocol

Output Requirement
- backup VM take over, keep output consistent with the primary has sent
  - by delay output until the backup VM receive all log entries prior to output operation, permit it replay until last log entry

Output Rule (help implement above)
- primary may not send any output, until the backup VM receives and **acks** log entries **associated with the output procedure**
  - Q: why include output log entry
  - A:
  > If the backup
were to go live at the point of the last log entry before the
output operation, some non-deterministic event (e.g. timer
interrupt delivered to the VM) might change its execution
path before it executed the output operation
    
    > If the backup VM has received all the log entries, including the log entry for the output-producing operation, then
the backup VM will be able to exactly reproduce the state
of the primary VM at that output point, and so if the primary dies, the backup will correctly reach a state that is consistent with that output

## 2.3 Detecting & Responding to Failure

How to infer failure happens?

VMware FT use UDP heartbeating between servers, regular timer interrupts, halt indicate failure
- failure could just be network lost between alive servers, so just let backup go live may caused split-brain

Q: 'GO LIVE' means?

A: stop replaying and take over as the primary VM.

Q: How to avoid split-brain?

A:
> we make use of the shared storage that
stores the virtual disks of the VM. When either a primary
or backup VM wants to go live, it executes an atomic **test-and-set** operation on the shared storage.

- Q: What is "an atomic test-and-set operation on the shared storage"?
- A: http://nil.csail.mit.edu/6.824/2020/papers/vm-ft-faq.txt
  - it seems both primary and backup will notice the network crash and want to go live? Even though the primary is 'live' actually, it will try test-and-set, fail then suicide.


# 3. Practical Implementation of FT

## 3.1 Starting FT VMs
what is the mechanism for starting a backup VM?

- needs?
  - can be apply to start a backup, and restart a VM after failure
  - useable for a running primary VM in arbitrary state

by using **FT VMotion**, modified from VM's Vmotion

- feature?
  - clone a VM to a remote host
  - set up a logging channel, souce->primary, destination-> new backup

## 3.2 Managing the logging channel
basic running logic of buffer in logging channel?
> The contents of the primary’s log buffer
are flushed out to the logging channel as soon as possible,
and log entries are read into the backup’s log buffer from the
logging channel as soon as they arrive

## 3.4 Implementation Issues for Disk IOs
How to solve non-de in DISKIO? non-de brought by the fact:
1. disk op can execute in parallel, simultaneous disk op access the same disk location bring non-de
2. VM's implementation of disk IO uses **DMA**, the simultaneous disk op access the same memory pages bring non-de

upper can be solve by detecting any such IO races, and force such racing disk ops to execute sequentially in the same way both primary and backup.

3. a disk op can race with a memory access by an application(or OS) in a VM
   - **e.g. if OS in a VM is reading a memory block, at the same time a disk read is occurring to that block**

   1. page protection (expensive to modify MMU)
      - when VM access the same page, there will be a trap, and VM can be paused until the disk operation completes.
   2. Bounce Buffer (cheaper)
      - a temporary buffer, same size as the memory being accessed by a disk operation
      - a disk read operation is modified to read the specified data to the bounce buffer, and the data is copied to guest memory only as the IO completion is delivered
      - a disk write, data sent first copied to the bounce buffer, and the disk write is modified to write data from the bounce buffer

> Q: How do Section 3.4's bounce buffers help avoid races?

>A: The problem arises when a network packet or requested disk block
arrives at the primary and needs to be copied into the primary's memory.
Without FT, the relevant hardware copies the data into memory while
software is executing. Guest instructions could read that memory
during the DMA; depending on exact timing, the guest might see or not
see the DMA'd data (this is the race). It would be bad if the primary
and backup both did this, but due to slight timing differences one
read just after the DMA and the other just before. In that case they
would diverge.
  - race发生在：网络那边告知disk的一块数据被更新了，然后用户正在读cache里这个块的数据，不确定他会不会读到
    - primary和backup就这个点的未知可能会diverge

>FT avoids this problem by not copying into guest memory while the
primary or backup is executing. FT first copies the network packet or
disk block into a private "bounce buffer" that the primary cannot
access. When this first copy completes, the FT hypervisor interrupts
the primary so that it is not executing. FT records the point at which
it interrupted the primary (as with any interrupt). Then FT copies the
bounce buffer into the primary's memory, and after that allows the
primary to continue executing. FT sends the data to the backup on the
log channel. The backup's FT interrupts the backup at the **same
instruction as the primary was interrupted** , copies the data into the
backup's memory while the backup is into executing, and then resumes
the backup.
   - 首先把网络过来的新信息，copy到弹跳栈里
   - 上面的copy完成后hypervisor中断掉primary的运行，把弹跳栈的内容copy到primary的cache里，然后放primary恢复运行
   - 上面这个中断的精准时刻被log，backup在相同时间也发生中断替换，所以不会diverge
   - TODO感觉这样处理的核心在于中断？不需要一个弹跳栈一样可以实现？

By this way, the exact timing of disk read/write is de

4. primary disk IO outstanding, failure happens, backup takes over (disk shared)

how to solve?

- no way for newly promoted primary to be sure if the disk IOs were issued to the disk, or completed successfully

no respond or return an error, let the guest try again?
- guest may not handle it form its local disk, not a good option

re-issue the pending IOs during the go-live process of the backup VM

> Q: Section 3.4 talks about disk I/Os that are outstanding on the
primary when a failure happens; it says "Instead, we re-issue the
pending I/Os during the go-live process of the backup VM." Where are
the pending I/Os located/stored, and how far back does the re-issuing
need to go?

> A: The paper is talking about disk I/Os for which there is a log entry
indicating the I/O was started but no entry indicating completion.
These are the I/O operations that must be re-started on the backup.
When an I/O completes, the I/O device generates an I/O completion
interrupt. So, if the I/O completion interrupt is missing in the log,
then the backup restarts the I/O. If there is an I/O completion
interrupt in the log, then there is no need to restart the I/O.

- [ ] how do you know there is a kind of entry indicating completion? Entry in log channels simiar to instructions?
  - just like no jump back instrution?

## 3.5 Implementation Issues for Network IO
- The biggest change to the networking emulation code for FT is the disabling of the asynchronous network optimizations, which may lead to non-de
  - code asynchronously updates VM ring buffers with incoming packets has been modified to force the guest to trap to the hypervisor, where it can log the updates and then apply them to the VM

obviously, disable asy brings performance lose, here comes the two optimizations of network:

1. implement clustering optimizations to reduce VM traps and interrupts
   - one transmit trap, per group of packets

2. reduce the delay for transmitted packets
   - Since hypervisor must delay all transmitted packets until it gets an ACK from the backup for the appropriate log entries
   - How? send, receive log entries & ACK can all be done without thread context switch
     - VMware vSphere hypervisor allows functions to be **registered** with the TCP stack, which can be called from a **deferred-execution** context whenever TCP data is received

# 4 Design Alternatives

## 4.1 Non-shared Disk
in the case of non-shared disks, the virutal disks are essentially considered part of the internal state of each VM. Therefore, disk writes of the primary do not have to be delayed according to the Output Rule
- useful in shared storage not accessible to the primary and backups, e.g. :
  1. expensive
  2. far away
- disadvantage
  1. explicitly sync, re-sync after failure
  2. no tiebreaker for split-brain, need extra

## 4.2 Executing Disk Reads on the Backup VM
default, backup never read from virtual disk, instead from logging ch
  
alter, read from disk
  - pros
    - reduce traffic on logging-ch
  - cons
    - slow down backup execution
    - extra work dealing failure in disk read

> Finally, there is a subtlety if this disk-read alternative is
used with the shared disk configuration. If the primary VM
does a read to a particular disk location, followed fairly soon
by a write to the same disk location, then the disk write
must be delayed until the backup VM has executed the first
disk read. This dependence can be detected and handled
correctly, but adds extra complexity.