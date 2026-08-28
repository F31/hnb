apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Chart.Name }}
  labels:
    app.kubernetes.io/name: {{ .Chart.Name }}
    app.kubernetes.io/instance: {{ .Release.Name }}
    app.kubernetes.io/component: {{ .Chart.Name }}
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      app.kubernetes.io/name: {{ .Chart.Name }}
      app.kubernetes.io/instance: {{ .Release.Name }}
  template:
    metadata:
      labels:
        app.kubernetes.io/name: {{ .Chart.Name }}
        app.kubernetes.io/instance: {{ .Release.Name }}
        app.kubernetes.io/component: {{ .Chart.Name }}
    spec:
      containers:
        - name: {{ .Chart.Name }}
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag | default $.Values.global.imageTag }}"
          imagePullPolicy: {{ .Values.image.pullPolicy | default $.Values.global.imagePullPolicy }}
          args:
            {{- if .Values.args }}
            {{- toYaml .Values.args | nindent 12 }}
            {{- end }}
          env:
            - name: NATS_URL
              value: {{ .Values.nats.url | default $.Values.global.nats.url }}
            - name: DB_DSN
              value: {{ .Values.db.dsn | default $.Values.global.postgresql.dsn }}
            - name: TUNNEL_TOKEN_SECRET
              valueFrom:
                secretKeyRef:
                  name: hnb-secrets
                  key: tunnel-token-secret
            - name: API_TOKEN_SECRET
              valueFrom:
                secretKeyRef:
                  name: hnb-secrets
                  key: api-token-secret
          ports:
            - containerPort: 8080
              name: http
            - containerPort: 9443
              name: tunnel
          resources:
            {{- toYaml .Values.resources | nindent 12 }}
          livenessProbe:
            httpGet:
              path: /health
              port: http
            initialDelaySeconds: 10
            periodSeconds: 30
          readinessProbe:
            httpGet:
              path: /ready
              port: http
            initialDelaySeconds: 5
            periodSeconds: 10
      serviceAccountName: {{ .Chart.Name }}

---
apiVersion: v1
kind: Service
metadata:
  name: {{ .Chart.Name }}
  labels:
    app.kubernetes.io/name: {{ .Chart.Name }}
    app.kubernetes.io/instance: {{ .Release.Name }}
spec:
  selector:
    app.kubernetes.io/name: {{ .Chart.Name }}
    app.kubernetes.io/instance: {{ .Release.Name }}
  ports:
    - port: {{ .Values.service.port | default 8080 }}
      targetPort: {{ .Values.service.port | default 8080 }}
      name: http
    {{- if .Values.service.tunnelPort }}
    - port: {{ .Values.service.tunnelPort }}
      targetPort: {{ .Values.service.tunnelPort }}
      name: tunnel
    {{- end }}