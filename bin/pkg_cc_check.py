#!/usr/bin/env python
#This script checks if every package satisfied package code coverage requirment.

helpinfo = "Usage: pkg_cc_check [cc report file] [cc requirement file]"

class PkgChecker:

    def __init__(self, cc_report, cc_requirements):
        self.cc_report = cc_report
        self.cc_requirements = cc_requirements

    def check(self):

        coverage = {}
        failed_pkgs = []

        try:
            with open(self.cc_report, 'r') as cc_report:
                for report in cc_report:
                    parts = report.split('\t')
                    if (len(parts) > 2) and (parts[0] != "?   "):
                        words = parts[-1].split(' ')
                        if len(words) > 2:
                            coverage[parts[1]] = words[1][:-1]
        except IOError as e:
            print ("Failed to read %s." %(self.cc_report), e)
            return 1

        try :
            with open(self.cc_requirements, 'r') as cc_requirements:
                for requirement in cc_requirements:
                    parts = requirement.split('\t')
                    if (len(parts) == 2):
                        if not coverage.has_key(parts[0]):
                            failed_pkgs.append(parts[0] + '\t' + "0.0" + '\t' + parts[1])
                        elif float(coverage[parts[0]]) < float(parts[1]):
                            failed_pkgs.append(parts[0] + '\t' + coverage[parts[0]] + '\t' + parts[1])
        except IOError as e:
            print ("Failed to read %s." %(self.cc_requirements), e)
            return 1


        if not failed_pkgs:
            print ("All packages passed code coverage requirement!")
        else:
            print ("Following package(s) failed to satisfied requirements:\n \
            Package Name    Actual Coverage    Requirement")
            for pkg in failed_pkgs:
                print pkg,
            print ("Failed to satisfied package coverage requirement(s)")
            return 2


if __name__ == "__main__":
    import sys
    if len(sys.argv) != 3:
        sys.exit("Wrong number of parameters \n" + helpinfo)
    pkgcheck = PkgChecker(sys.argv[1], sys.argv[2])
    sys.exit(pkgcheck.check())

