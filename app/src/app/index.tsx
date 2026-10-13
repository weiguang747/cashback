import { router } from 'expo-router';
import { useState } from 'react';
import { KeyboardAvoidingView, Platform, StyleSheet, View } from 'react-native';

import { Button, Icon, Screen, Text, TextField } from '@/components';
import * as api from '@/lib/api';
import { colors, radii, spacing } from '@/theme';

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/** Sign in — demo only, no password. */
export default function SignInScreen() {
  const [email, setEmail] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const valid = EMAIL.test(email.trim());

  const signIn = async () => {
    if (!valid || loading) return;
    setLoading(true);
    setError(null);
    try {
      // TODO: store the token once the server issues real ones.
      await api.signIn(email.trim());
      router.replace('/home');
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong. Please try again.');
    } finally {
      setLoading(false);
    }
  };

  const onChangeEmail = (text: string) => {
    setEmail(text);
    setError(null);
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
            onChangeText={onChangeEmail}
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
          {error && (
            <Text variant="caption" color="danger" accessibilityRole="alert" style={styles.error}>
              {error}
            </Text>
          )}
        </View>

        <View style={styles.flex} />

        <Button label="Continue" onPress={signIn} disabled={!valid} loading={loading} />
        <Text variant="caption" color="textMuted" align="center" style={styles.note}>
          Demo App
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
  error: { marginTop: spacing.sm },
  note: { marginTop: spacing.lg },
});
