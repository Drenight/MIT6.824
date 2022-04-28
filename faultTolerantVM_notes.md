# ABSTRACT
enterprise-grade fault-tolerant virtual machines
- replicating the execution of primary VM via backup VM on another server

<<<<<<< HEAD
# 1. Introduction
=======
# 1 Introduction
>>>>>>> e2572b4 (FTVM)
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
