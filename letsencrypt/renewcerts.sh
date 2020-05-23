#!/bin/bash

if [[ -z $EMAIL || -z $DOMAINS || -z $SECRET || -z $DEPLOYMENT ]]; then
	echo "EMAIL, DOMAINS, SECERT, and DEPLOYMENT env vars required"
	env
	exit 1
fi

NAMESPACE=$(cat /var/run/secrets/kubernetes.io/serviceaccount/namespace)

certbot certonly \
  --dns-google \
  --dns-google-propagation-seconds 120 \
  --email ${EMAIL} \
  --agree-tos \
  --non-interactive \
  --config-dir /home/certbot/config \
  --work-dir /home/certbot/work \
  --logs-dir /home/certbot/logs \
  -d ${DOMAINS} || exit 1

CERTPATH=/home/certbot/config/live/$(echo $DOMAINS | cut -f1 -d',')

ls $CERTPATH || exit 1

cat secret-patch-template.json | \
	sed "s/NAMESPACE/${NAMESPACE}/" | \
	sed "s/NAME/${SECRET}/" | \
	sed "s/TLSCERT/$(cat ${CERTPATH}/fullchain.pem | base64 | tr -d '\n')/" | \
	sed "s/TLSKEY/$(cat ${CERTPATH}/privkey.pem |  base64 | tr -d '\n')/" \
	> secret-patch.json

ls secret-patch.json || exit 1

# update secret
curl -v --cacert /var/run/secrets/kubernetes.io/serviceaccount/ca.crt -H "Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)" -k -v -XPATCH  -H "Accept: application/json, */*" -H "Content-Type: application/strategic-merge-patch+json" -d @secret-patch.json https://kubernetes/api/v1/namespaces/${NAMESPACE}/secrets/${SECRET}

cat deployment-patch-template.json | \
	sed "s/TLSUPDATED/$(date)/" | \
	sed "s/NAMESPACE/${NAMESPACE}/" | \
	sed "s/NAME/${DEPLOYMENT}/" \
	> deployment-patch.json

ls deployment-patch.json || exit 1

# update pod spec on ingress deployment to trigger redeploy
curl -v --cacert /var/run/secrets/kubernetes.io/serviceaccount/ca.crt -H "Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)" -k -v -XPATCH  -H "Accept: application/json, */*" -H "Content-Type: application/strategic-merge-patch+json" -d @deployment-patch.json https://kubernetes/apis/extensions/v1beta1/namespaces/${NAMESPACE}/ingresses/${DEPLOYMENT}

