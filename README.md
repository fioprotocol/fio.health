# fio.health

Provides API and P2P health checks for FIO nodes. It has limited reporting ability, outputing static content
with testing results. The purpose behind this is that it allows running from AWS lambda, across multiple regions,
while remaining entirely in the free tier for lambda and cloudwatch, only requiring storage and bandwidth costs which
should be minimal. It can also be run via cron on a server, it will automatically detect which environment is used.

### API Checks:

 - basic connectivity (records duration for first connection)
 - correct chain id
 - expected version
 - database row lookups, fio request lookups, producer schedule
 - head block time lag
 - roundtrip time to server
 - weak TLS versions supported, and weak ciphers
 - if CORS is set to be permissive
 - if any potentially dangerous nodeos plugins are enabled.

There is some limited historical information provided as a chart for response times, and head block lag, click on the
small chart icon next to current response time, and head block lag in the report.

### P2P Checks:

 - If node is reachable
 - Chain ID is correct
 - Waits for next block to be sent.
 - Reports head block time lag

### Alerting

Alerts can be sent to a telegram group. The API key is only accepted as an environment variable `TELEGRAM`
(it's likely that if using S3 that this could be public, and would be bad to expose the key.)

Expects two variables to be set in config.yml if using telegram:
 - The group / channel, `telegram_channel`uses the @groupname format
 - The url for the health report `base_url`

The FIO mainnet notification group is available at: https://t.me/fiohealthnotify

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

Requires the [GeoLite2 Country database](https://www.maxmind.com/), but **does not** supply the file, visit the geolite site
for instructions on obtaining the database files, or it's possible they are already included in a package for your
operating system. If building for lambda, copy this file into the `dist` directory, and run `make lambda` which will
create `dist/deployment.zip` containing the executable `main` (set "main" as the lambda entrypoint)

## Misc notes

TLS checks are relatively permissive, it will alarm on TLS v1.0, and very weak ciphers, it does not currently alarm on
BEAST etc. While it would be easy to add, most browsers have long-since added mitigations for these attacks.


