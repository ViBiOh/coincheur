# Coincheur

Application macOS qui compte les points de coinche en regardant les cartes à la caméra : on montre un éventail ou des cartes étalées, l'appli reconnaît d'un coup les cartes visibles et affiche le score des deux équipes.

> **Vibecodé de l'enfer.** Ce projet a été écrit presque entièrement par une IA, au fil de l'eau, pour arrêter de compter les points à la main le midi. Il n'a pas été relu avec rigueur, la détection est approximative et la correction manuelle reste nécessaire. Ne l'utilisez pas pour quoi que ce soit de sérieux.

## Principe

1. On démarre le scan : l'aperçu de la caméra s'affiche à gauche avec les cartes reconnues encadrées, la liste des cartes vues à droite.
2. On montre les cartes de l'équipe qui a le moins de plis, en éventail ou étalées. Une carte est validée quand elle est vue sur deux images de suite.
3. On corrige à la main dans la grille des 32 cartes si besoin. Une carte retirée à la main ne revient pas toute seule.
4. On choisit l'atout, puis qui a le 10 de der et la belote/rebelote.
5. L'appli affiche les points des deux équipes. L'autre équipe est déduite : 152 moins les cartes scannées, plus 10 de der et 20 de belote.
6. Si la détection se trompe, le bouton « Signaler une erreur » enregistre l'image de la caméra en pleine résolution avec un fichier JSON décrivant ce que l'appli avait vu (cartes validées, cadres et scores, réglages). Les rapports sont dans le dossier de configuration de l'utilisateur (`~/Library/Application Support/coincheur/reports`, modifiable avec `COINCHEUR_REPORT_DIR`), et « Ouvrir le dossier des erreurs » l'ouvre dans le Finder. Ces images servent à analyser puis à réentraîner le modèle.
7. Chaque équipe remporte des plis entiers de 4 cartes : une fois le scan arrêté, si le nombre de cartes lues n'est pas un multiple de 4, un avertissement s'affiche.

Les capots, contrats, coinches et le calcul du gagnant ne sont pas gérés.

## Architecture

Application [Wails v2](https://wails.io) : la logique est en Go, l'interface est une webview légère qui affiche l'état.

| Dossier | Rôle |
| --- | --- |
| `internal/camera` | capture de la caméra par AVFoundation (cgo, Objective C), ne garde que la dernière image |
| `internal/detect` | ONNX Runtime avec CoreML : prétraitement, décodage, suppression des doublons |
| `internal/scan` | boucle image par image, validation des cartes sur plusieurs images, aperçu JPEG |
| `internal/report` | enregistrement d'une image de la caméra et de ce que l'appli en avait compris |
| `internal/scoring` | points des cartes, calcul du score, règle du multiple de 4 |
| `frontend` | interface en TypeScript (Vite), sans framework |
| `training` | génération des images synthétiques et entraînement du modèle de détection |
| `cmd/evaluate` | outil en ligne de commande : détection sur des photos JPEG, pour comparer des modèles |

## Modèle de détection

Un YOLOv8s qui détecte les coins des cartes (valeur et couleur) plutôt que les cartes entières, ce qui permet de lire un éventail dont seuls les coins dépassent. Il est entraîné sur des images synthétiques (voir `training/`) à partir de jeux de cartes libres de droits :

- [AustinGabriel/Public-Domain-and-CC0-Playing-Cards](https://github.com/AustinGabriel/Public-Domain-and-CC0-Playing-Cards) (CC0)
- [saulspatz/SVGCards](https://github.com/saulspatz/SVGCards) (domaine public)
- [seeligd/svg-playing-cards](https://github.com/seeligd/svg-playing-cards) (domaine public)

## Installation

Prérequis : macOS sur Apple Silicon, Go, Node et [uv](https://docs.astral.sh/uv/). La CLI Wails et `mockgen` sont déclarés comme outils Go dans `go.mod` et s'utilisent par `go tool`, sans installation.

```sh
make deps
make dev
```

`make deps` (ou séparément `make ort`, `make model` et `make frontend-deps`) télécharge ONNX Runtime dans `build/lib/`, exporte `models/corners.pt` vers `build/models/` et installe les dépendances du frontend. Les emplacements se changent par les variables `COINCHEUR_ORT_LIB` et `COINCHEUR_MODEL`. Le fournisseur de calcul se règle avec `COINCHEUR_PROVIDER` (`coreml` ou `cpu`) et `COINCHEUR_UNITS` (`ALL`, `CPUAndGPU`, `CPUAndNeuralEngine`).

Pour produire l'application :

```sh
make build
```

La bibliothèque ONNX Runtime et le modèle sont copiés dans le paquet `.app` (`Contents/Frameworks` et `Contents/Resources`). Au premier démarrage du scan, macOS demande l'autorisation d'utiliser la caméra.

## Entraînement

Les poids entraînés sont versionnés dans `models/corners.pt` (22 Mo). `make model` les exporte en ONNX vers `build/models/`. Pour entraîner un nouveau modèle :

```sh
./training/fetch-assets.sh
cd training
uv run --no-project --with opencv-python-headless --with numpy --with pillow python generate.py --train 5000 --val 500
uv run --no-project --with ultralytics --with onnx --with onnxslim python train.py --epochs 30 --imgsz 832
cp runs/corners/weights/best.pt ../models/corners.pt
```

`fetch-assets.sh` télécharge les trois jeux de cartes et rend les SVG en PNG (il faut `rsvg-convert`, par `brew install librsvg`). L'entraînement prend plusieurs heures sur un Mac. Il dépend d'Ultralytics et n'est pas nécessaire pour utiliser l'application.

## Outils

- `make test` : tests Go avec le détecteur de courses et vérification des types du frontend
- `make lint` : `golangci-lint`
- `make evaluate` puis `./build/evaluate photo.jpg` : cartes détectées sur une photo, avec le temps par image

## Limites

- Les cartes très recouvertes ou à l'envers dans un éventail peuvent être ratées. La correction manuelle est là pour ça.
- Les couleurs proches (coeur et carreau, pique et trèfle) se confondent parfois, vérifiez toujours la grille.
- L'entraînement synthétique ne connaît pas votre jeu : une police ou une impression très différente peut dégrader la détection.

## Licence

Le code et les poids du modèle sont publiés sous licence [GNU AGPL-3.0](LICENSE). Le modèle est dérivé de YOLOv8 d'Ultralytics, lui même sous AGPL-3.0, ce qui impose ce choix pour les poids (`models/corners.pt`). Les images d'entraînement sont générées à partir de jeux de cartes sous licence CC0 ou du domaine public (voir plus haut).
