// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Ebitengine Authors

#include <stddef.h>
#include <string.h>

static char buf[256];

const char *join_strings(const char **strs) {
    buf[0] = '\0';
    if (strs == NULL) {
        return "(null)";
    }
    for (size_t i = 0; strs[i] != NULL; i++) {
        if (i > 0) {
            strcat(buf, ",");
        }
        strcat(buf, strs[i]);
    }
    return buf;
}

static const char *items[] = {"alpha", "beta", "gamma", NULL};

const char **get_strings(int empty) {
    if (empty) {
        return NULL;
    }
    return items;
}

float sum_floats4(const float v[4]) {
    return v[0] + v[1] + v[2] + v[3];
}
