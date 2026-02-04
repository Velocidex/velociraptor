package process

/*

Tracking processes is not as simple as it first appears. The
complicating factor is that in many systems, the process ID is not
globally unique and (e.g. on Windows) this is reused aggressively.

One option is to use a GUID to represent processes. However, usually
the user wants to resolve the PID provided by various OS calls into a
previous record. This means the PID is a foreign key.

To implement this we use a double dispatch LRU:

1. A `Link Entry` is a Process Entry with the RealId field pointing to
   a globally unique process ID.

2. Process Entries themselves use a globally unique ID - this is
   formed by joining the ID with the start time of the process. These
   real Process IDs do not change.

3. When a process entry is added, we check if there is an existing
   link to the Pid and resolve the real process ID to it.


*/
