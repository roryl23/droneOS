#include <Arduino.h>

// function declarations
int myFunction(int, int);

void setup() {
  int result = myFunction(2, 3);
}

void loop() {
}

// function definitions
int myFunction(int x, int y) {
  return x + y;
}