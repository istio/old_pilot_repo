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
    initTestingCluster(utils)
    stage('Bazel Build') {
      // Empty kube/config file signals to use in-cluster auto-configuration
      sh('ln -s ~/.kube/config platform/kube/')
      sh('bin/install-prereqs.sh')
      bazel.fetch('-k //...')
      bazel.build('//...')
    }
    stage('Go Build') {
      sh('bin/init.sh')
    }
    stage('Code Check') {
      sh('bin/check.sh')
    }
    stage('Bazel Tests') {
      bazel.test('//...')
    }
    stage('Code Coverage') {
      sh('bin/codecov.sh')
      utils.publishCodeCoverage('MANAGER_CODECOV_TOKEN')
    }
    stage('Integration Tests') {
      timeout(15) {
        sh('bin/e2e.sh -tag alpha' + gitUtils.GIT_SHA + ' -v 2')
      }
    }
  }
}

def initTestingCluster(utils) {
  def cluster = utils.failIfNullOrEmpty(env.E2E_CLUSTER, 'E2E_CLUSTER is not set')
  def zone = utils.failIfNullOrEmpty(env.E2E_CLUSTER_ZONE, 'E2E_CLUSTER_ZONE is not set')
  def project = utils.failIfNullOrEmpty(env.PROJECT, 'PROJECT is not set')
  sh('gcloud config set container/use_client_certificate True')
  sh("gcloud container clusters get-credentials " +
      "--project ${project} --zone ${zone} ${cluster}")

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
        sh('bin/e2e.sh -count 10 -debug -tag alpha' + gitUtils.GIT_SHA + ' -v 2')
      }
    }
  }
}
