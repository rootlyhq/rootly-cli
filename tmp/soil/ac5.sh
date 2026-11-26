#!/bin/bash
cd /workspace/lane
B=57534c36cac6a220d788256f8d627b31784c9a68
echo '$ git diff --stat <base>...HEAD'; git diff --stat $B...HEAD
echo; echo '$ call sites of statuspagewarn in non-test code'
grep -rn "statuspagewarn\." --include=*.go internal | grep -v _test.go | grep -v "^internal/statuspagewarn/"
echo; echo '$ added lines in cmd files that add flags or change Use/Aliases (expect none)'
git diff $B...HEAD -- internal/cmd | grep -E '^\+' | grep -E 'Flags\(\)\.|MarkFlag|Use:|Aliases:' || echo "none"
echo; echo '$ HTTP methods used by the new package (expect no PUT/PATCH/POST/Delete/Update/Resolve)'
grep -nE 'Update|Resolve[A-Z]|Create|Delete|MethodP|MethodDelete' internal/statuspagewarn/statuspagewarn.go || echo "none"
echo; echo '$ api calls in statuspagewarn.go'
grep -n "client\." internal/statuspagewarn/statuspagewarn.go
