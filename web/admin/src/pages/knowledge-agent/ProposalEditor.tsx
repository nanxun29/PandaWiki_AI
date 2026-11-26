import { Editor, EditorMarkdown, useTiptap } from '@ctzhian/tiptap';
import { Box, Button, Stack } from '@mui/material';
import { useState } from 'react';
import DiffViewer from 'react-diff-viewer';

export default function ProposalEditor({
  before,
  content,
  contentType,
  onChange,
}: {
  before: string;
  content: string;
  contentType: string;
  onChange: (value: string) => void;
}) {
  const [diff, setDiff] = useState(true);
  const isMarkdown = contentType === 'md';
  const editor = useTiptap({
    content,
    contentType: isMarkdown ? 'markdown' : 'html',
    editable: !isMarkdown,
    immediatelyRender: true,
    exclude: ['invisibleCharacters', 'youtube', 'mention'],
    onUpdate: ({ editor: current }) => {
      if (!isMarkdown) onChange(current.getHTML());
    },
  });
  return (
    <Stack gap={2}>
      <Stack direction='row' gap={1}>
        <Button
          variant={diff ? 'contained' : 'outlined'}
          onClick={() => setDiff(true)}
        >
          查看差异
        </Button>
        <Button
          variant={!diff ? 'contained' : 'outlined'}
          onClick={() => setDiff(false)}
        >
          编辑建议正文
        </Button>
      </Stack>
      {diff ? (
        <Box sx={{ overflow: 'auto' }}>
          <DiffViewer oldValue={before} newValue={content} splitView={false} />
        </Box>
      ) : isMarkdown ? (
        <EditorMarkdown
          editor={editor.editor}
          value={content}
          onAceChange={onChange}
          height='400px'
        />
      ) : (
        <Box>
          <Editor editor={editor.editor} />
        </Box>
      )}
    </Stack>
  );
}
