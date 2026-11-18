// Same rule as the server (auth.ValidPassword): 6 to 128 characters with an uppercase letter,
// a lowercase letter and a digit.
export const validPassword = (p: string) => /^(?=.*[a-z])(?=.*[A-Z])(?=.*[0-9]).{6,128}$/.test(p);
