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
