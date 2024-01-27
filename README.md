# Pushgateway Upsetter

The Pushgateway Upsetter polls a Prometheus [Pushgateway][] and automatically
adds an `up` metric to groups with a `job` / `instance` label set.

[pushgateway]: https://github.com/prometheus/pushgateway

## Usage

```
$ ./upsetter --help
Usage of ./upsetter:
  -refresh string
        Refresh period (default "20s")
  -url string
        Pushgateway URL (default "http://localhost:9091")
```
