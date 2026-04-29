cd /home/demo/work/work_ai/video_prj/RiVision/rivision_prj/rivision-k3-deploy/install_dir/rivision_server/rivision_server/inference-gateway-go && go build -o inference-gateway ./cmd/main.go 
sudo systemctl stop rivision-gateway &&  cp inference-gateway /opt/rivision-gateway/rivision-gateway && sudo systemctl restart rivision-gateway
