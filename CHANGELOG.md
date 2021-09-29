## 0.4.0 / 2021-10-05
* [ENHANCEMENT] Support HTTP POST for /api/v1/labels endpoint. #70
* [FEATURE] Add `--error-on-replace` flag (defaults to `false`) to return an error if a label value would otherwise be siltently replaced. #67
* [ENHANCEMENT] Add label enforce support for the new query_exemplars API. #65

## 0.3.0 / 2021-04-16

* [FEATURE] Add support for /api/v1/series, /api/v1/labels and /api/v1/label/<name>/values endpoints (Prometheus/Thanos). #49
* [FEATURE] Add `-passthrough-paths` flag (empty by default), which allows exposing chosen resources from upstream without enforcing (e.g Prometheus UI). #48
* [ENHANCEMENT] Add support for queries via HTTP POST. #53

## 0.2.0 / 2020-10-08

* [FEATURE] Add support for /api/v1/rules (Prometheus/Thanos). #16
* [FEATURE] Add support for /api/v1/alerts (Prometheus/Thanos). #18
* [FEATURE] Add support for /api/v2/silences (Alertmanager). #20
* [ENHANCEMENT] Enforce validity of the `-label` and `-upstream` CLI arguments. #33
* [ENHANCEMENT] Allow multiple enforcement matchers. #39
* [BUGFIX] Decompress gzipped response if needed. #35

## 0.1.0 / 2018-10-24

Initial release.
