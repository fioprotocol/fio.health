# fio.health

Basic health checks for FIO nodes. At present it has limited reporting ability, only outputing a static html file
with details from the last test run.

### API Checks:

 - basic connectivity
 - several basic lookups work correctly
 - head block time lag
 - roundtrip time to server
 - weak TLS versions supported, and weak ciphers
 - if CORS is set to be permissive
 - if any potentially dangerous nodeos plugins are enabled.

### P2P Checks:

 - If node is reachable
 - Chain ID is correct
 - Waits for next block to be sent.
 - Reports head block time lag

### Configuration:

Uses a yaml file to specify options, see [example-config.yml](./example-config.yml) for the format.
There are only two runtime options:

```
Usage of ./fio-health:
  -config string
    	yaml config file to load, can be local file, or S3 uri, or ENV var: CONFIG (default "config.yml")
  -db string
    	geo lite database to open (default "GeoLite2-Country.mmdb")
```

### Deploying:

Will work as either a standalone tool or is capable of running from AWS lambda. If using S3 it will not ask for api
information, you must configure the environment variables, AWS configuration files, or provide a role to the instance
or lambda function.

Requires the [GeoLite2 Country database](https://www.maxmind.com/), but does not supply the file, visit the geolite site
for instructions on obtaining the database files, or it's possible they are already included in a package for your
operating system. If building for lambda, copy this file into the `dist` directory, and run `make lambda` which will
create `dist/deployment.zip` containing the executable `main` (set "main" as the lambda entrypoint)

### Future:

For now this only outuputs a static HTML file and any needed assets. Future plans include alerting via telegram, streaming
event data to cloudwatch, or exporting .json with detailed results for each run. A second module is planned that will
perform transactions to ensure end-to-end functionality is present.

A Terraform plan for easy deployment is also planned.

