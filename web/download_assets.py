import urllib.request
import os
from pathlib import Path

base_url = "https://raw.githubusercontent.com/mrdoob/three.js/master/examples/textures/"
target_dir = Path(__file__).resolve().parent / "public" / "textures"

files = {
    "waternormals.jpg": "waternormals.jpg",
    "skybox/px.jpg": "cube/MilkyWay/dark-s_px.jpg",
    "skybox/nx.jpg": "cube/MilkyWay/dark-s_nx.jpg",
    "skybox/py.jpg": "cube/MilkyWay/dark-s_py.jpg",
    "skybox/ny.jpg": "cube/MilkyWay/dark-s_ny.jpg",
    "skybox/pz.jpg": "cube/MilkyWay/dark-s_pz.jpg",
    "skybox/nz.jpg": "cube/MilkyWay/dark-s_nz.jpg",
}

os.makedirs(target_dir / "skybox", exist_ok=True)

for local, remote in files.items():
    print(f"Downloading {local}...")
    try:
        urllib.request.urlretrieve(base_url + remote, target_dir / local)
    except Exception as e:
        print(f"Failed to download {local}: {e}")
