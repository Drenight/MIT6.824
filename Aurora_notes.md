# Abstract 

- quick skim about Aurora

What's Aurora?
- a relational database service
- for OLTP(Online Transaction Processing) workloads

Assumption about "What's the bottle-neck in high throughput data processing"
- compute & storage $\rightarrow$ network, since 2017

What Aurora do focusing network optimization?
- A novel architecture
  - push **redo processing** to a multi-tenant scale-out(水平扩展) storage service(who is purpose-built for Aurora)
    - not only reduce network traffic
    - but also allows for fast crash recovery

# 1 Introduction

Cloud?
- Many IT workloads are moving to public cloud providers

Cloud service 

# 2 Durability at Scale

# 3 The log is the Database

# 4 The log marches forward

# 5 Putting it all together

# 6 Performance results

# 7 Lessons learned

# 8 Related work

# 9 Conclusion

# 10 Acknowledgments

# 11 Reference