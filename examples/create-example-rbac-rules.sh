#!/bin/bash

kubectl delete ns namespace1
kubectl delete ns namespace2
kubectl delete clusterrole my-clusterrole
kubectl delete clusterrole my-clusterrole2
kubectl delete clusterrole orphaned-clusterrole
kubectl delete clusterrole psp-clusterrole
kubectl delete clusterrolebinding my-clusterrole-binding
kubectl delete clusterrolebinding my-clusterrole-binding2
kubectl delete clusterrolebinding my-clusterrole-binding3
kubectl delete clusterrolebinding psp-clusterrole-binding

kubectl create ns namespace1
kubectl create ns namespace2

kubectl -n namespace1 create serviceaccount orphaned-service-account
kubectl -n namespace1 create serviceaccount my-service-account
kubectl -n namespace1 create serviceaccount my-service-account2
kubectl -n namespace2 create serviceaccount my-service-account
kubectl -n namespace2 create serviceaccount my-service-account2

kubectl create clusterrole my-clusterrole --verb=get --resource=services
kubectl create clusterrole my-clusterrole2 --verb=get --resource=endpoints
kubectl create clusterrole psp-clusterrole --verb=use --resource=podsecuritypolicies --resource-name=privileged
kubectl create clusterrole orphaned-clusterrole --verb=create,get,update --resource=secrets
kubectl create clusterrolebinding my-clusterrole-binding --clusterrole my-clusterrole --serviceaccount namespace1:my-service-account2
kubectl create clusterrolebinding psp-clusterrole-binding --clusterrole psp-clusterrole --serviceaccount namespace1:my-service-account
kubectl create clusterrolebinding my-clusterrole-binding2 --clusterrole my-clusterrole2 --user user2 --group group2
kubectl create clusterrolebinding my-clusterrole-binding3 --clusterrole missing-clusterrole --serviceaccount namespace1:missing-service-account

kubectl -n namespace1 create role my-role --verb=get --verb=list --resource=pods
kubectl -n namespace1 create role my-role2 --verb=get --resource=deployments.apps --resource-name=my-name
kubectl -n namespace1 create role orphaned-role --verb=get --resource=statefulsets.apps
kubectl -n namespace1 create rolebinding my-role-binding --role my-role --serviceaccount namespace1:my-service-account --user user1 --group group1
kubectl -n namespace1 create rolebinding my-role-binding2 --role my-role --serviceaccount namespace1:my-service-account2 --serviceaccount namespace2:my-service-account
kubectl -n namespace1 create rolebinding my-role-binding3 --role my-role2 --serviceaccount namespace2:my-service-account
kubectl -n namespace1 create rolebinding my-role-binding4 --clusterrole my-clusterrole --serviceaccount namespace1:my-service-account
kubectl -n namespace1 create rolebinding my-role-binding5 --role missing-role --serviceaccount namespace1:missing-service-account
kubectl -n namespace1 create rolebinding my-role-binding6 --clusterrole missing-clusterrole --serviceaccount namespace1:my-service-account
kubectl -n namespace1 create rolebinding my-role-binding7 --role my-role

kubectl -n namespace2 create role my-role --verb=get --verb=list --resource=pods
kubectl -n namespace2 create role my-role2 --verb=get --resource=deployments.apps --resource-name=my-name
kubectl -n namespace2 create role orphaned-role --verb=get --resource=statefulsets.apps
kubectl -n namespace2 create rolebinding my-role-binding --role my-role --serviceaccount namespace2:my-service-account --user user1 --group group1
kubectl -n namespace2 create rolebinding my-role-binding2 --role my-role --serviceaccount namespace2:my-service-account2 --serviceaccount namespace1:my-service-account
kubectl -n namespace2 create rolebinding my-role-binding3 --role my-role2 --serviceaccount namespace2:my-service-account
kubectl -n namespace2 create rolebinding my-role-binding4 --clusterrole my-clusterrole --serviceaccount namespace2:my-service-account

