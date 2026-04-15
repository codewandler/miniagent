# Benchmark: File Operation Pipeline
EXPECTED: success

Complete all of the following steps. If every step succeeds, write the single
word "success" to /tmp/bench_result.txt. If any step fails, write "failure".

Steps (batch them into as few commands as possible):
  1. Create the directory /tmp/bench_b002
  2. Write the text "hello bench" into /tmp/bench_b002/test.txt
  3. Read /tmp/bench_b002/test.txt and confirm it contains "hello bench"
  4. Delete /tmp/bench_b002 and everything inside it
  5. Confirm /tmp/bench_b002 no longer exists

Write "success" or "failure" to /tmp/bench_result.txt.
