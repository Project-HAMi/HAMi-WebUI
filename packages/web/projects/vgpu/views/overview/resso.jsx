export default function resso({ color }) {
  return (
    <div
      class="color"
      style={{
        background: color,
        width: '6px',
        height: '6px',
        'border-radius': '2px',
      }}
    />
  );
}
