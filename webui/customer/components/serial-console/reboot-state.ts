export function getPostRebootFailureState() {
  return {
    shouldDisconnect: false,
    shouldReconnect: false,
    shouldKeepRebooting: false,
  };
}

export function getPostRebootSuccessState() {
  return {
    shouldDisconnect: true,
    shouldReconnect: true,
    shouldKeepRebooting: true,
  };
}
