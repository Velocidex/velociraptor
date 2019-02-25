# Setting up monitoring.

To set up monitoring for Velociraptor servers you will need to install
both Graphana and Prometheus.

## Setting up Prometheus

Prometheus will scrape the Velociraptor server monitoring port and
record time series of critical server state. You can use the provided
prometheus.yaml file without change and launch prometheus like this:

```
$ prometheus  --config.file velociraptor.yml
```

## Setting up Graphana

Graphana is a graphing package which helps to visualize the Prometheus
data. Graphana is very easy to use and can be configured using its web
interface.

First add Prometheus as a data source.

You can import the graphana.json file as a new dashboard. This is a
good starting point for a useful monitoring dashboard and will show
the following graphs:

1. Flow completion rate in flows/second - this gives an indication of
   active hunts and how busy the server is.

2. Client current connections - shows how many clients are currently
   connected to the server.

3. Resident memory size - shows how much memory the server is using.

4. CPU Load - shows how much cpu the server is currently using.
