import Mock from 'mockjs';

const Random = Mock.Random;

Random.extend({
  format: function (date) {
    var format = [
      { name: 'Maven', icon: 'maven' },
      { name: 'npm', icon: 'npm' },
      { name: 'Go', icon: 'go' },
      { name: 'PyPi', icon: 'pypi' },
      { name: 'RPM', icon: 'rpm' },
      { name: 'Debian', icon: 'debian' },
      { name: 'Conan', icon: 'conan' },
      { name: 'NuGet', icon: 'nuget' },
      { name: 'Docker', icon: 'docker' },
      { name: 'Helm', icon: 'helm' },
      { name: 'Raw', icon: 'raw' },
    ];
    return this.pick(format.map((item) => item.icon));
  },
});
Random.format();

Mock.mock('/repoList', 'get', () => {
  return Mock.mock({
    status: 200,
    'list|1-10': [
      {
        name: `@first`,
        format: '@format',
        url: '@url',
        time: '@time',
      },
    ],
  });
});

Mock.mock('/mock/repoProsList', 'post', ({ body }) => {
  const { repoId } = JSON.parse(body);

  return Mock.mock({
    status: 200,
    'list|1-10': [
      {
        name: `${repoId}-制品-@first`,
      },
    ],
  });
});
