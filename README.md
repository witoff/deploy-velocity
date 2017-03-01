Deploy Velocity
===============

"Ship and iterate.  The companies that are the fastest at this process will win."

**Local Development:**
```bash
make run-local

# Debug against a single site
go run src/* -d -v -u https://google.com
```

**Deploy:**
```bash
make deploy
# Invoke
serverless invoke -f main
# Monitor
serverless logs --tail -f main
```
