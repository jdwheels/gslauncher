# gslauncher

Backend for a webapp that scales up and down an ECS cluster for self hosted game servers. Fargate was not used due to the unique performance requirements of the servers. 

These servers would be used infrequently, but needed to be performant and available on demand.

It also supports Server Side Events for sending realtime updates to the client.
