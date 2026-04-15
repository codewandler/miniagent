# Benchmark: Error Recovery
EXPECTED: caught

Run the following command which will fail:
  cat /absolutely/nonexistent/path/file_xyz_bench.txt

Catch the error gracefully. Do NOT let the error crash your response.

Write the word "caught" followed by a brief description of the error to
/tmp/bench_result.txt. Example:
  caught: No such file or directory

The file /tmp/bench_result.txt must exist and start with the word "caught".
