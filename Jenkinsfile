label = "${UUID.randomUUID().toString()}"
git_project = "app-resource-scaler"
git_project_user = "v3io"
git_project_upstream_user = "iguazio"
git_deploy_user = "iguazio-prod-git-user"
git_deploy_user_token = "iguazio-prod-git-user-token"
git_deploy_user_private_key = "iguazio-prod-git-user-private-key"

podTemplate(label: "${git_project}-${label}", inheritFrom: "jnlp-docker") {
    node("${git_project}-${label}") {
        pipelinex = library(identifier: 'pipelinex@development', retriever: modernSCM(
                [$class       : 'GitSCMSource',
                 credentialsId: git_deploy_user_private_key,
                 remote       : "git@github.com:iguazio/pipelinex.git"])).com.iguazio.pipelinex
        common.notify_slack {
            withCredentials([
                    string(credentialsId: git_deploy_user_token, variable: 'GIT_TOKEN')
            ]) {
            multi_credentials=[pipelinex.DockerRepo.ARTIFACTORY_IGUAZIO, pipelinex.DockerRepo.DOCKER_HUB, pipelinex.DockerRepo.QUAY_IO, pipelinex.DockerRepo.GCR_IO]

                github.release(git_deploy_user, git_project, git_project_user, git_project_upstream_user, true, GIT_TOKEN) {
                    stage("build ${git_project} in dood") {
                        container('docker-cmd') {
                            dir("${github.BUILD_FOLDER}/src/github.com/${git_project_upstream_user}/${git_project}") {
                                sh("SCALER_TAG=${github.DOCKER_TAG_VERSION} SCALER_REPOSITORY='' make build")
                            }
                        }
                    }

                    stage('push') {
                        container('docker-cmd') {
                            dockerx.images_push_multi_registries(["autoscaler:${github.DOCKER_TAG_VERSION}", "dlx:${github.DOCKER_TAG_VERSION}"], multi_credentials)
                        }
                    }
                }
            }
        }
    }
}
