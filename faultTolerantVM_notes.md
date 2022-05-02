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
challenges for replicating execution of running VM
1. capturing all input and non-de to ensure de excution
2. apply input and non-de to backup
3. performance

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
