#!/usr/bin/env python

import sys

coverage = {}
failed_pkgs = []

try :
    with open('cc_report', 'r') as cc_report :
        for report in cc_report :
            parts = report.split('\t')
            if (len(parts) > 2) and (parts[0] != "?   ") :
                words = parts[-1].split(' ')
                if len(words) > 2 :
                    coverage[parts[1]] = words[1][:-1]
except IOError :
    sys.exit("Failed to read cc_report.")

try :
    with open('cc_requirements') as cc_requirements :
        for requirement in cc_requirements :
            parts = requirement.split('\t')
            if (len(parts) == 2) :
                if not coverage.has_key(parts[0]) :
                    failed_pkgs.append(parts[0] + '\t' + "0.0" + '\t' + parts[1])
                elif float(coverage[parts[0]]) < float(parts[1]) :
                    failed_pkgs.append(parts[0] + '\t' + coverage[parts[0]] + '\t' + parts[1])
except IOError :
    sys.exit("Failed to read cc_requirements.")


if len(failed_pkgs) == 0 :
    print "All packages pass code coverage requirement!"
else :
    print "Following package(s) failed to satisfied requirements:\n \
    Package Name    Actual Coverage    Requirement"
    for pkg in failed_pkgs :
        print pkg,
    sys.exit("Failed to satisfied package coverage requirement(s)")

