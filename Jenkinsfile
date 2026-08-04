// Jenkinsfile — Pipeline GitFlow para finanzas_personales.
//
// Flujo GitFlow soportado (Multibranch Pipeline + GitHub Branch Source):
//   feature/*  ──PR──▶  develop  ──release/*──▶  main
//   hotfix/*   ──PR──▶  main
//
// - Las PRs (feature/hotfix) se VALIDAN en Jenkins antes del merge: gate rápido
//   (vet + build) y gate de cobertura 85%. El estado del check aparece en la PR.
// - develop y release/* validan y despliegan a staging.
// - main valida y despliega a producción con aprobación manual (input).
//
// Patrones de protección aplicados:
//   - `when { changeset }`: CI solo corre cuando cambian archivos relevantes
//     (Go, módulos o migraciones); PRs de solo documentación se saltean.
//   - `options`: timeout anti-hangs, timestamps y retención de builds.
//   - Gate de cobertura 85% (scripts/coverage.sh): el build FALLA si la
//     cobertura baja del umbral.
//   - Credencial `finanzas_database_url` inyecta DATABASE_URL para MySQL.
//     Sin MySQL, los tests de integración se saltean y la cobertura cae a ~46%,
//     por lo que el gate protege la medición.
//
// Configuración de Jenkins (una vez):
//   1. Job tipo "Multibranch Pipeline", fuente GitHub con el repo.
//   2. "Branch Sources" que incluya ramas (`feature/*`, `develop`, `release/*`,
//      `hotfix/*`, `main`) y "Discover pull requests".
//   3. Credential de tipo "Secret text" con id `finanzas_database_url` con el
//      DSN completo (ej. el de .env.example). Contiene `&`, se inyecta tal cual.
//   4. En GitHub: "Require status checks" para el check del pipeline en main.

pipeline {
    agent { label 'golang' }

    options {
        timeout(time: 20, unit: 'MINUTES')
        timestamps()
        disableConcurrentBuilds()
        buildDiscarder(logRotator(numToKeepStr: '30', daysToKeepStr: '30'))
    }

    environment {
        MIN_COVERAGE = '85'
        GOFLAGS = '-mod=mod'
    }

    stages {
        stage('Validación (CI)') {
            // Corresponde al "gate" de gitflow: la PR no se mergea sin pasar.
            when {
                anyOf(
                    changeRequest(),                          // PRs: feature/* → develop, hotfix/* → main
                    branch 'develop',
                    branch 'main',
                    branch pattern: 'release/*',
                    branch pattern: 'hotfix/*'
                )
                changeset pattern: ['**/*.go', 'go.mod', 'go.sum', 'migrations/*.sql']
            }
            stages {
                stage('Gate rápido (vet + build)') {
                    steps {
                        sh 'go version'
                        sh 'go vet ./...'
                        sh 'go build ./...'
                    }
                }
                stage('Tests + gate de cobertura 85%') {
                    steps {
                        // Los tests de integración (cmd/server/integration_test.go)
                        // usan TEST_DATABASE_URL/DATABASE_URL (ver .env.example).
                        withCredentials([string(credentialsId: 'finanzas_database_url', variable: 'DATABASE_URL')]) {
                            sh 'MIN_COVERAGE=${MIN_COVERAGE} ./scripts/coverage.sh'
                        }
                    }
                }
            }
        }

        stage('Deploy staging') {
            // gitflow: develop integra features; release/* es la rama de staging
            // que se promociona a main. TODO: reemplazar con el deploy real
            // (ej. make docker-build + push a registro de staging).
            when { anyOf(branch 'develop', branch pattern: 'release/*') }
            steps {
                echo "Deploy a staging desde ${env.BRANCH_NAME}"
            }
        }

        stage('Deploy producción') {
            // gitflow: main es producción. Aprobación manual antes de desplegar.
            // TODO: reemplazar con el deploy real a producción.
            when { branch 'main' }
            steps {
                input message: '¿Aprobar deploy a producción?', ok: 'Deploy'
                echo "Deploy a producción desde main"
            }
        }
    }

    post {
        always {
            archiveArtifacts artifacts: 'coverage.html, coverage.out, coverage.filtered.out', allowEmptyArchive: true
            cleanWs()
        }
        failure {
            echo "Pipeline falló en ${env.BRANCH_NAME}${env.CHANGE_ID ? ' (PR #' + env.CHANGE_ID + ')' : ''}"
        }
    }
}
