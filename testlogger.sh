#!/bin/sh
# tests whether rclone check and rclone sync output exactly the same file lists.
RCLONE="go run rclone.go"
ROOT="_junk"
SRC=${ROOT}/src
DST=${ROOT}/dst

echo "Filling src and dst with makefiles:"
$RCLONE test makefiles $SRC --seed 0
$RCLONE test makefiles $DST --seed 1
$RCLONE test makefiles $SRC --seed 2
$RCLONE test makefiles $DST --seed 2

echo "running rclone check for baseline test:"
# (error is expected here, ignore it)
$RCLONE check $SRC $DST --match $ROOT/CHECKmatchfile.txt --combined $ROOT/CHECKcombinedfile.txt --missing-on-src $ROOT/CHECKmissingonsrcfile.txt --missing-on-dst $ROOT/CHECKmissingondstfile.txt --error $ROOT/CHECKerrfile.txt --differ $ROOT/CHECKdifferfile.txt  || true

echo "running sync with output files:"
$RCLONE sync $SRC $DST --match $ROOT/SYNCmatchfile.txt --combined $ROOT/SYNCcombinedfile.txt --missing-on-src $ROOT/SYNCmissingonsrcfile.txt --missing-on-dst $ROOT/SYNCmissingondstfile.txt --error $ROOT/SYNCerrfile.txt --differ $ROOT/SYNCdifferfile.txt

echo "sorting them by line and diffing:"
sort $ROOT/CHECKmatchfile.txt -o $ROOT/CHECKmatchfile.txt
sort $ROOT/CHECKcombinedfile.txt -o $ROOT/CHECKcombinedfile.txt
sort $ROOT/CHECKmissingonsrcfile.txt -o $ROOT/CHECKmissingonsrcfile.txt
sort $ROOT/CHECKmissingondstfile.txt -o $ROOT/CHECKmissingondstfile.txt
sort $ROOT/CHECKerrfile.txt -o $ROOT/CHECKerrfile.txt
sort $ROOT/CHECKdifferfile.txt -o $ROOT/CHECKdifferfile.txt

sort $ROOT/SYNCmatchfile.txt -o $ROOT/SYNCmatchfile.txt
sort $ROOT/SYNCcombinedfile.txt -o $ROOT/SYNCcombinedfile.txt
sort $ROOT/SYNCmissingonsrcfile.txt -o $ROOT/SYNCmissingonsrcfile.txt
sort $ROOT/SYNCmissingondstfile.txt -o $ROOT/SYNCmissingondstfile.txt
sort $ROOT/SYNCerrfile.txt -o $ROOT/SYNCerrfile.txt
sort $ROOT/SYNCdifferfile.txt -o $ROOT/SYNCdifferfile.txt

echo "diff match check vs. sync:"
diff $ROOT/CHECKmatchfile.txt $ROOT/SYNCmatchfile.txt
echo "diff combined check vs. sync:"
diff $ROOT/CHECKcombinedfile.txt $ROOT/SYNCcombinedfile.txt
echo "diff missingonsrc check vs. sync:"
diff $ROOT/CHECKmissingonsrcfile.txt $ROOT/SYNCmissingonsrcfile.txt
echo "diff missingondst check vs. sync:"
diff $ROOT/CHECKmissingondstfile.txt $ROOT/SYNCmissingondstfile.txt
echo "diff error check vs. sync:"
diff $ROOT/CHECKerrfile.txt $ROOT/SYNCerrfile.txt
echo "diff differ check vs. sync:"
diff $ROOT/CHECKdifferfile.txt $ROOT/SYNCdifferfile.txt