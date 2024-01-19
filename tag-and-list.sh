# get an argument with the tag name
TAG=$1

# ensure tag is not empty
if [ -z "$TAG" ]; then
    echo "Usage: $0 <tag>"
    exit 1
fi

git add .
git commit -m "blah"
git tag $TAG
git push origin $TAG
GOPROXY=proxy.golang.org go list -m github.com/nguyendon/gosoap@${TAG}
