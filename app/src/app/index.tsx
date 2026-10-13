import { router } from 'expo-router';
import { useState } from 'react';
import { KeyboardAvoidingView, Platform, StyleSheet, View } from 'react-native';

import { Button, Icon, Screen, Text, TextField } from '@/components';
import { colors, radii, spacing } from '@/theme';

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/** Sign in — demo only, no password. */
export default function SignInScreen() {
  const [email, setEmail] = useState('');
  const valid = EMAIL.test(email.trim());

  const signIn = () => {
    if (valid) router.replace('/home');
  };

  return (
    <Screen>
      <KeyboardAvoidingView style={styles.flex} behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
        <View style={styles.logo}>
          <Icon name="card" size={26} color={colors.reward} />
        </View>

        <Text variant="display" accessibilityRole="header" style={styles.title}>
          Sign in
        </Text>
        <Text variant="body" color="textMuted" style={styles.subtitle}>
          Enter your email to continue.
        </Text>

        <View style={styles.field}>
          <TextField
            label="Email"
            value={email}
            onChangeText={setEmail}
            placeholder="you@example.com"
            keyboardType="email-address"
            autoCapitalize="none"
            autoComplete="email"
            autoCorrect={false}
            textContentType="emailAddress"
            returnKeyType="go"
            onSubmitEditing={signIn}
            valid={valid}
          />
        </View>

        <View style={styles.flex} />

        <Button label="Continue" onPress={signIn} disabled={!valid} />
        <Text variant="caption" color="textMuted" align="center" style={styles.note}>
          Demo app · no password needed
        </Text>
      </KeyboardAvoidingView>
    </Screen>
  );
}

const styles = StyleSheet.create({
  flex: { flex: 1 },
  logo: {
    marginTop: spacing.xxxl,
    width: 56,
    height: 56,
    borderRadius: radii.xl,
    backgroundColor: colors.brand,
    alignItems: 'center',
    justifyContent: 'center',
  },
  title: { marginTop: spacing.xxxl },
  subtitle: { marginTop: spacing.sm },
  field: { marginTop: spacing.xxxl },
  note: { marginTop: spacing.lg },
});
