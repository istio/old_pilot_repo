#!/usr/bin/env python

import os
import unittest
import pkg_cc_check

class PkgCheckTestCase(unittest.TestCase):

    tmp_report = 'pkg_check_tmp-cc_report'
    tmp_requirements = 'pkg_check_tmp-cc_requirements'

    def setUp(self):
        self.cleanTempFiles()
        self.pkgChecker = pkg_cc_check.PkgChecker(self.tmp_report, self.tmp_requirements)

    def tearDown(self):
        self.cleanTempFiles()

    def cleanTempFiles(self):
        try:
            os.remove(self.tmp_report)
            os.remove(self.tmp_requirements)
        except OSError:
            pass

    def testParseReport(self):
        line = "?   \tmanager/cmd\t[no test files]"
        self.pkgChecker.parseReport(line)
        self.assertEqual(self.pkgChecker.coverage, {})

        line = "ok  \tmanager/model\t1.3s\tcoverage: 90.2% of statements"
        self.pkgChecker.parseReport(line)
        self.assertEqual(self.pkgChecker.coverage, {'manager/model': '90.2'})

    def testcompRequire(self):
        failed_pkgs = []
        self.pkgChecker.coverage = {'manager/model': '90.2', 'manager/cmd': '80.5'}
        line = "manager/model\t90.1"
        self.pkgChecker.compRequire(line, failed_pkgs)
        self.assertFalse(failed_pkgs)

        line = "manager/cmd\t85"
        self.pkgChecker.compRequire(line, failed_pkgs)
        self.assertEqual(len(failed_pkgs), 1)

    def testCheck(self):
        self.assertEqual(self.pkgChecker.check(), 1)
        with open (self.tmp_report, 'w') as f:
            f.writelines("ok  \tmanager/model\t1.3s\tcoverage: 90.2% of statements")
        self.assertEqual(self.pkgChecker.check(), 1)
        with open (self.tmp_requirements, 'w') as f:
            f.writelines("manager/model\t85.6")
        self.assertEqual(self.pkgChecker.check(), 0)
        with open (self.tmp_requirements, 'w') as f:
            f.writelines("manager/model\t95.6")
        self.assertEqual(self.pkgChecker.check(), 2)

if __name__ == "__main__":
    unittest.main()
