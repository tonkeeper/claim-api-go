#!groovy

pipeline {
  agent any
  options {
    timestamps()
    disableConcurrentBuilds()
  }
  environment {
    TG_TOKEN = credentials('tg_bot_zaya_monitoring')
    TG_CHAT_ID = credentials('tg_chan_tonapi_build')
  }
  // this is going to trigger the build on webhook
  // if building like that
  // _changes to this file will take effect on a second build_
  // as they were not present in jenkins on the moment of trigger
  triggers {
    GenericTrigger(
     genericVariables: [[key: 'REF', value: '$.ref']],
     causeString: 'Triggered on tag $REF',
     regexpFilterExpression: '^v[0-9].*',
     regexpFilterText: '$REF',
     printContributedVariables: true,
     printPostContent: false,
     token: 'claim-api-Aexok2ohsei4'
    )
  }
  stages {
    stage('checkout') {
      steps {
        script {
          env.APP_GIT_REPO = 'git@github.com:tonkeeper/claim-api-go.git'
          env.APP_GIT_TAG = '' // so the var is defined but empty on a manual build

          if (env.REF) {
            env.APP_GIT_BRANCH = env.REF
            env.APP_GIT_TAG = env.REF
          } else {
            env.APP_GIT_BRANCH = 'master'
          }

          env.APP_DOCKER_IMAGE_NAME = 'claim-api'

          def scmVars = checkout([
                          $class: 'GitSCM',
                          userRemoteConfigs: [[
                            url: "${APP_GIT_REPO}",
                            credentialsId: 'tonkeeper-build-bot_ssh'
                          ]],
                          branches: [[name: "${APP_GIT_BRANCH}"]],
                          extensions: [[
                            $class: 'SubmoduleOption',
                            disableSubmodules: false,
                            parentCredentials: true,
                            recursiveSubmodules: true,
                            reference: '',
                            shallow: true,
                            trackingSubmodules: true
                          ]]
                        ])
          env.APP_GIT_COMMIT = scmVars.GIT_COMMIT[0..6]
          env.APP_BUILD_DATE = sh(script: "date --rfc-3339=seconds --utc", returnStdout: true).trim()
          env.APP_DOCKER_REPO_TAG = "${APP_GIT_TAG ? APP_GIT_TAG : APP_GIT_COMMIT}"
        }
        sh """
           docker build . \
           --label org.opencontainers.image.source="${APP_GIT_REPO}" \
           --label org.opencontainers.image.created="${APP_BUILD_DATE}" \
           --label org.opencontainers.image.revision="${APP_GIT_COMMIT}" \
           --label docker.repo.tag="${APP_DOCKER_REPO_TAG}" \
           --tag "${DOCKER_REPOSITORY}/${APP_DOCKER_IMAGE_NAME}:${APP_DOCKER_REPO_TAG}" \
           --tag "${DOCKER_REPOSITORY}/${APP_DOCKER_IMAGE_NAME}:${APP_GIT_COMMIT}" \
           --tag "${DOCKER_REPOSITORY}/${APP_DOCKER_IMAGE_NAME}:latest"
           docker push "${DOCKER_REPOSITORY}/${APP_DOCKER_IMAGE_NAME}:${APP_DOCKER_REPO_TAG}"
           docker push "${DOCKER_REPOSITORY}/${APP_DOCKER_IMAGE_NAME}:${APP_GIT_COMMIT}"
           docker push "${DOCKER_REPOSITORY}/${APP_DOCKER_IMAGE_NAME}:latest"
           """
      }
    }
//    stage('docker travesty') {
//      steps { sshagent(['builder']) {
//          sh """
//             ssh -o StrictHostKeyChecking=no ubuntu@claim-api.tonapi.io '
//               sudo /usr/bin/docker compose --project-directory /opt/claim-api pull &&
//               sudo /usr/bin/docker compose --project-directory /opt/claim-api up -d '
//             """
//        } } }
  }
  post {
    success {
      sh '''
      curl -s https://api.telegram.org/bot${TG_TOKEN}/sendMessage \
      --request POST --header 'Content-Type: multipart/form-data' \
      --form chat_id=${TG_CHAT_ID} \
      --form text="${APP_DOCKER_IMAGE_NAME}:${APP_GIT_BRANCH} OK"
      '''
    }
    failure {
      sh '''
      curl -s https://api.telegram.org/bot${TG_TOKEN}/sendMessage \
      --request POST --header 'Content-Type: multipart/form-data' \
      --form chat_id=${TG_CHAT_ID} \
      --form text="${APP_DOCKER_IMAGE_NAME}:${APP_GIT_BRANCH} FAIL"
      '''
    }
  }
}
