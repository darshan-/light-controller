#include <fcntl.h>
#include <stdlib.h>
#include <stdio.h>

#include <linux/input.h>
#include <string.h>
#include <stdint.h>

void usage ( int argc, char *argv[] )
{
    printf("Usage:\n\t%s key\n" , argv[0]);

    exit(EXIT_FAILURE);
}

int main ( int argc, char *argv[], char *env[] )
{
    if ( argc != 2 )    usage(argc, argv);

    int key;

    if ( strcmp(argv[1], "lshift") == 0 )       key = KEY_LEFTSHIFT;
    else if ( strcmp(argv[1], "rshift") == 0 )  key = KEY_RIGHTSHIFT;
    else if ( strcmp(argv[1], "lalt") == 0 )    key = KEY_LEFTALT;
    else if ( strcmp(argv[1], "ralt") == 0 )    key = KEY_RIGHTALT;
    else if ( strcmp(argv[1], "lctrl") == 0 )   key = KEY_LEFTCTRL;
    else if ( strcmp(argv[1], "rctrl") == 0 )   key = KEY_RIGHTCTRL;

    printf("A1\n");

    //FILE *kbd = fopen("/dev/input/event0", "r");
    //if (kbd == NULL) {
    //    printf("kbd is NULL; cuold not fopen device file!\n");
    //    return 1;
    //}
    //int fd = fileno(kbd);

    int fd = open("/dev/input/event0", O_RDONLY);
    printf("fd: %d\n", fd);
    if (fd < 0) {
        printf("Could not open device file!\n");
        return 1;
    }


    // KEY_MAX = 0x2ff;

    printf("A2\n");
    //char key_map[KEY_MAX/8 + 1];    //  Create a byte array the size of the number of keys
    uint8_t key_map[KEY_MAX/8 + 1];

    printf("3\n");
    //memset(key_map, 0, sizeof(key_map));    //  Initate the array to zero's
    memset(key_map, 0, sizeof(key_map));
    printf("4\n");
    //ioctl(fileno(kbd), EVIOCGKEY(sizeof(key_map)), key_map);    //  Fill the keymap with the current keyboard state
    ioctl(fd, EVIOCGKEY(sizeof(key_map)), key_map);
    printf("5\n");

    int keyb = key_map[key/8];  //  The key we want (and the seven others arround it)
    printf("6\n");
    int mask = 1 << (key % 8);  //  Put a one in the same column as out key state will be in;
    printf("7\n");

    return !(keyb & mask);  //  Returns true if pressed otherwise false

}
