'use client';

import { Box, styled } from '@mui/material';

// Shared with the existing Wiki Q&A surface and the authenticated management assistant.
export const StyledUserBubble = styled(Box)(({ theme }) => ({
  alignSelf: 'flex-end',
  maxWidth: '75%',
  padding: theme.spacing(1, 2),
  borderRadius: '10px 10px 0px 10px',
  backgroundColor: theme.palette.primary.main,
  color: theme.palette.primary.contrastText,
  fontSize: 14,
  wordBreak: 'break-word',
}));

export const StyledAiBubble = styled(Box)(({ theme }) => ({
  alignSelf: 'flex-start',
  display: 'flex',
  flexDirection: 'column',
  width: '100%',
  gap: theme.spacing(3),
}));

export const StyledAiBubbleContent = styled(Box)(() => ({
  wordBreak: 'break-word',
}));
