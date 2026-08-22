<!--
  Stands in for QRScanner in component tests: the real one reaches for
  getUserMedia on mount, which jsdom hasn't got. It accepts the same
  callbacks so a test can drive a decode without a camera.
-->
<script lang="ts">
    interface Props {
        onDecoded?: (text: string) => void;
        onError?: (message: string) => void;
    }
    let { onDecoded, onError }: Props = $props();
</script>

<div
    data-testid="qr-stub"
    data-wired={onDecoded && onError ? "both" : "partial"}
></div>
