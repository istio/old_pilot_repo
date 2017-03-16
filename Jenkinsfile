#!groovy

@Library('testutils@stable-838b134')

import org.istio.testutils.Utilities
import org.istio.testutils.GitUtilities
import org.istio.testutils.Bazel

// Utilities shared amongst modules
def gitUtils = new GitUtilities()
def utils = new Utilities()
def bazel = new Bazel()

mainFlow(utils) {
  pullRequest(utils) {

    node {
      gitUtils.initialize()
      bazel.setVars()
    }

    if (utils.runStage('PRESUBMIT')) {
      presubmit(gitUtils, bazel, utils)
    }

    if (utils.runStage('POSTSUBMIT')) {
      postsubmit(gitUtils, bazel, utils)
    }
  }
}

def presubmit(gitUtils, bazel, utils) {
  goBuildNode(gitUtils, 'istio.io/manager') {
    bazel.updateBazelRc()
    stage('Bazel Build') {
      // Empty kube/config file signals to use in-cluster auto-configuration
      sh('touch platform/kube/config')
      sh('toolbox/scripts/install-prereqs.sh')
      bazel.fetch('-k //...')
      bazel.build('//...')
    }
    stage('Go Build') {
      sh('toolbox/scripts/init.sh')
    }
    stage('Code Check') {
      sh('toolbox/scripts/check.sh')
    }
    stage('Bazel Tests') {
      bazel.test('//...')
    }
    stage('Code Coverage') {
      sh('toolbox/scripts/codecov.sh')
      utils.publishCodeCoverage('MANAGER_CODECOV_TOKEN')
    }
    stage('Integration Tests') {
      timeout(15) {
        sh('toolbox/scripts/e2e.sh -tag alpha' + gitUtils.GIT_SHA + ' -v 2')
      }
    }
  }
}


def postsubmit(gitUtils, bazel, utils) {
  buildNode(gitUtils) {
    stage('Docker Push') {
      bazel.updateBazelRc()
      def images = 'init,init_debug,app,app_debug,runtime,runtime_debug'
      def tags = "${gitUtils.GIT_SHORT_SHA},\$(date +%Y-%m-%d-%H.%M.%S),latest"
      utils.publishDockerImages(images, tags)
    }
    stage('Integration Tests') {
      // Empty kube/config file signals to use in-cluster auto-configuration
      sh('touch platform/kube/config')
      timeout(30) {
        sh('toolbox/scripts/e2e.sh -count 10 -debug -tag alpha' + gitUtils.GIT_SHA + ' -v 2')
      }
    }
  }
}
