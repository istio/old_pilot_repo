#!/bin/bash
cat <<EOF
package mixer

func GlobalList() ([]string) { 
  tmp := make([]string, len(globalList))
  copy(tmp, globalList)
  return tmp
}

var (
  globalList = []string{
EOF

cat - |\
  sed '/^#/d' |\
  sed '/^\s*$/d' |\
  sed 's/^- \(.*\)$/    "\1",/'

cat <<EOF
  }
)
EOF
