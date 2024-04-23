## 0.1.3 (April, 23, 2024)

SECURITY:

* Update dependencies to fix security vulnerabilities.
Due to the large number of dependencies, the list of CVEs is not included in this release note. [[GH-44](https://github.com/hashicorp/consul/issues/44)]

BUG FIXES:

* Fix a bug that would cause AWS instance discovery queries to fail if the CloudMap Namespace name was different from it's HTTP Name.
This can happen when a CloudMap Namespace is re-created with the same Name. [[GH-46](https://github.com/hashicorp/consul/issues/46)]

## 0.1.2 (April 7, 2020)

IMPROVEMENTS:

*  Do not create Cloud Map services with A+SRV DNS records [[GH-20](https://github.com/hashicorp/consul-aws/pull/20)]

## 0.1.1 (December 20, 2018)

IMPROVEMENTS:

* Use the new DiscoverInstances API [[GH-2](https://github.com/hashicorp/consul-aws/pull/2)]
* Introduce `-aws-poll-interval` and deprecate `-aws-pull-interval` [[GH-4](https://github.com/hashicorp/consul-aws/pull/4)]

BUG FIXES:

* Fix an issue where prefixes are not handled properly [[GH-2](https://github.com/hashicorp/consul-aws/pull/2)]

## 0.1.0 (November 29, 2018)

* Initial release
