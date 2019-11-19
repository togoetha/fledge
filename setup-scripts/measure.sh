#!/bin/bash

if [[ -z "${3:-}" ]]; then
  echo "Use: measure.sh period iterations csv-pids"
  exit 1
fi

period=${1}
shift
iterations=${1}
shift
csvpids=${1}

#pids=$(echo $csvpids | tr ";" "")
IFS="," read -ra pids <<< "$csvpids"

iter=0

while [ $iter -lt $iterations ]
#echo "Iteration $iter"
do
  line=""
  for pid in "${pids[@]}"
  do
#    echo "Pid $pid"
    memuse=$(pmap -X $pid | tail -n 1 | awk '{ print $3 }')
    line="$line;$memuse"
  done
  echo $line
  sleep $period
  iter=$[$iter+1]
done
