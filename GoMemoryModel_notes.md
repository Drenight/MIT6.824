https://go.dev/ref/mem

# Intro
specify the conditions, reads of variable in one goroutine can observe values produced by writes to the same variable in a different goroutine

# Advice
serialize simulatneous access from multi goroutines, channel or sync

# "Happens Before"
compiler & processor can reorder read&write executed within a single goroutinue
- only when reording do not change behavior defined by language

happens before:
> a partial order, event e1 happens before event e2

- a read r of a variable v is **allowed** to observe a write w to v if:
  1. r does not happen before w
  2. no other w' to v happens after w but before r

- To guarantee a read r **observes a particular write** w if:
  1. w happens before r
  2. Any other write to the shared variable v either happens before w or after r

stronger than the first pair: no other wirtes **concurrently** with w or r, but same under single goroutine

- must use synchronization events to establish "happens-before" with multiple goroutinues access

# Synchronization

## Initialization
>If a package p imports package q, the completion of q's init functions happens before the start of any of p's.

> The start of the function main.main happens after all init functions have finished.