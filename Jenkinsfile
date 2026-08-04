// Jenkinsfile — Pipeline de CI para finanzas_personales.
//
// Patrones de protección aplicados:
//   - `when { changeset }`: CI solo corre cuando cambian archivos relevantes
//     (Go, módulos o migraciones); PRs de solo documentación se saltean.
//   - `options`: timeout anti-hangs, timestamps, serialización de builds
//     concurrentes sobre la misma rama y retención de builds (buildDiscarder).
//   - Gate rápido: `go vet` + `go build` fallan temprano antes de correr tests.
//   - Gate de cobertura 85% (scripts/coverage.sh): el build FALLA si la
//     cobertura baja del umbral.
//   - Credencial `finanzas_database_url` inyecta DATABASE_URL para MySQL.
//     Si MySQL no está disponible, los tests de integración se saltean y la
//     cobertura cae a ~46%, por lo que el gate de 85% protege la medición.
//   - Reportes: JUnit para resultados de tests y coverage.html como artefacto.

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
        stage('Relevancia del cambio') {
            // Corta el pipeline si el PR no toca código Go, módulos o migraciones.
            when {
                changeset pattern: ['**/*.go', 'go.mod', 'go.sum', 'migrations/*.sql']
            }
            steps {
                echo 'Cambios relevantes detectados: se ejecuta el pipeline completo.'
            }
        }

        stage('Gate rápido (vet + build)') {
            when {
                changeset pattern: ['**/*.go', 'go.mod', 'go.sum', 'migrations/*.sql']
            }
            steps {
                sh 'go version'
                sh 'go vet ./...'
                sh 'go build ./...'
            }
        }

        stage('Tests + gate de cobertura') {
            when {
                changeset pattern: ['**/*.go', 'go.mod', 'go.sum', 'migrations/*.sql']
            }
            steps {
                // Los tests de integración (cmd/server/integration_test.go) usan
                // TEST_DATABASE_URL/DATABASE_URL. En Jenkins se inyecta vía credential.
                withCredentials([string(credentialsId: 'finanzas_database_url', variable: 'DATABASE_URL')]) {
                    sh 'MIN_COVERAGE=${MIN_COVERAGE} ./scripts/coverage.sh'
                }
            }
        }

        stage('Reportes') {
            when {
                changeset pattern: ['**/*.go', 'go.mod', 'go.sum', 'migrations/*.sql']
            }
            steps {
                sh '''
                    set +e
                    go test ./... -coverpkg=./... -coverprofile=coverage.out 2>&1 | tee test-output.txt
                    rc=${PIPESTATUS[0]}
                    set -e
                    exit $rc
                '''
                sh 'go install github.com/jstemmer/go-junit-report/v2@latest'
                sh 'cat test-output.txt | go-junit-report > junit.xml'
                junit allowEmptyResults: true, testResults: 'junit.xml'
            }
        }
    }

    post {
        always {
            archiveArtifacts artifacts: 'coverage.html, coverage.out, coverage.filtered.out, test-output.txt', allowEmptyArchive: true
            cleanWs()
        }
    }
}
