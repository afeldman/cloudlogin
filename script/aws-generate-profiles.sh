#!/usr/bin/env bash

REGION="eu-central-1"
START_URL="https://lynqtech.awsapps.com/start"
CONFIG="$HOME/.aws/config"

TOKEN=$(jq -r --arg URL "$START_URL" \
'. | select(.startUrl==$URL) | .accessToken' \
~/.aws/sso/cache/*.json)

if [ -z "$TOKEN" ]; then
  echo "No valid SSO token found. Run:"
  echo "aws sso login"
  exit 1
fi

ACCOUNTS=$(aws sso list-accounts --access-token "$TOKEN" --region "$REGION")

for ID in $(echo "$ACCOUNTS" | jq -r '.accountList[].accountId'); do
  NAME=$(echo "$ACCOUNTS" | jq -r ".accountList[] | select(.accountId==\"$ID\") | .accountName")

  ROLES=$(aws sso list-account-roles \
    --access-token "$TOKEN" \
    --account-id "$ID" \
    --region "$REGION")

  for ROLE in $(echo "$ROLES" | jq -r '.roleList[].roleName'); do
    PROFILE=$(echo "$NAME-$ROLE" | tr ' ' '-' | tr '[:upper:]' '[:lower:]')

cat >> "$CONFIG" <<EOF

[profile $PROFILE]
sso_start_url = $START_URL
sso_region = $REGION
sso_account_id = $ID
sso_role_name = $ROLE
region = $REGION
output = json
EOF

  done
done