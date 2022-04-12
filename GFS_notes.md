<<<<<<< HEAD
=======
# 2. Design Overview

>>>>>>> 85a1cb5 (GFS notes added)
## 2.4 Single Master

### interactions for a simple read
1. client sends master (fileName, chunk index), caculated by fixed chunk size and byte offset
2. master reply in (chunk handle, chunk locations of replicas), client caches.
3. client sends one of replicas(closest) in (chunk handle, byte range)

## 2.5 Chunk Size
> However, hot spots did develop when GFS was first used

Q:seems big chunk size brings hotspot problem?

A:
[stackOverflow related question](https://stackoverflow.com/questions/46577706/why-do-small-files-create-hot-spots-in-the-google-file-system)
In simple terms, if the chunk size is lower, the chance of **simultaneous** access to the same chunk is lower, under the scenario of 1000 clients & 10 servers. All clients will become not completely synchronized due to nature of network, clients' working load, etc; Another explaination is that the paper's point is to say in practical application, not all clients end up reading the entire file at all. It's not about chunk size's business.

## 2.6 Metadata

### three major (stored in memory so pretty quick)
  1. file and chunk namespaces
  2. mapping from files to chunks
  3. locations of each chunk's replicas

- capacity whole system limited by master's memory(metadata is in memory), prefix compression, 64bytes expressing 64MB chunk
- operation log, lose it lose all, replicate remote, respond client's operation after flush to disk re&lo
-  checkpoint, B-tree

## 2.7 Consistency Model (GFS's guarantees)
- File namespace(e.g., file creation) are atomic
- write, record appends(atomically at least once)
- successful mutations -> file region is **guaranteed** to be defined and contain the last mutation, by
  1. apply mutas all replicas in same order
  2. chunk version prevent stale record
<<<<<<< HEAD
=======
- cache purge
- checksum for bad chunkservers

### APP's accommodate
- practically mutate files by appending > overwriting (more efficient and resilient for failures), checkpoints allow restart incrementally

# 3. System Interactions

## 3.1 Leases and Mutation Order
- each mutation -> all chunk's replicas
- the chunk got lease -> the primary, who can pick a serial order for all mutations, other replicas follow this order, request piggybacked in HeartBeat to extend
- revoke by master, e.g. disable mutations on a file being renamed

### interactions for a simple write
1. client asks master which chunkserver holds the currenct lease for the chunk and the locations of other replicas
2. master replies (identity of primary, locations of secondary replicas), client caches it
3. client pushes data to all relicas(any order), chunkserver store the data in an LRU buffer until the data is used or aged out(LAZY).
4. after all replicas ack, client sends write req to primary, primary assigns consecutive serial numbers to mutations received from multiple clients, apply mutation to its own local state
5. primary forwards write req to all secondary
6. all secondaries reply to primary, they done
7. primary reply client, any error in any replicas reported, retry from 3->7
>>>>>>> 85a1cb5 (GFS notes added)
