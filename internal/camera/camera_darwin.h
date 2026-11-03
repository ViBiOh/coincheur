#ifndef CAMERA_DARWIN_H
#define CAMERA_DARWIN_H

#include <stdint.h>

int cameraStart(uintptr_t handle, char **errorMessage);
void cameraStop(void);

#endif
