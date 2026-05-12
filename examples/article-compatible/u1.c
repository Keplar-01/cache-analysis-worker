int main() {
    int i;
    int a[256];
    int b[256];
    int c[256];

    for (i = 0; i < 256; i = i + 1) {
        a[i] = b[i] + c[i] * 2;
    }

    return 0;
}