const normalizeText = (value) =>
  typeof value === 'string' ? value.trim() : '';

export const buildNodeDetailLocation = ({ uid, nodeName } = {}) => {
  const normalizedUid = normalizeText(uid);
  if (!normalizedUid) return null;

  const normalizedNodeName = normalizeText(nodeName);
  const location = {
    name: 'node-admin-detail',
    params: { uid: normalizedUid },
  };

  if (normalizedNodeName) {
    location.query = { nodeName: normalizedNodeName };
  }

  return location;
};
