int main() {
    int i;
    int a[2048];
    int b[2048];

    for (i = 0; i < 1024; i = i + 1) {
        a[i * 2] = b[i * 2];
    }

    return 0;
}