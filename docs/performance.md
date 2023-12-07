# About performance

- Protobuf is wonderful, and this is the default for fuyuu-router (however, it may be due to an issue with Go's standard json library). JSON is convenient for debugging.

- In my benchmark (100MB HTTP request/response), there is not much difference between zstd and none. The implementation method or benchmark method might be incorrect.