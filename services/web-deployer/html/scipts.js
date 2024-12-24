document.addEventListener('DOMContentLoaded', () => {
    const ctx = document.getElementById('temperatureChart').getContext('2d');
    const temperatureChart = new Chart(ctx, {
        type: 'line',
        data: {
            labels: ['January', 'February', 'March', 'April', 'May', 'June'],
            datasets: [{
                label: 'Average Temperature (°C)',
                data: [22, 19, 23, 25, 20, 18],
                borderColor: 'rgba(75, 192, 192, 1)',
                backgroundColor: 'rgba(75, 192, 192, 0.2)',
                fill: true,
            }]
        },
        options: {
            responsive: true,
            plugins: {
                title: {
                    display: true,
                    text: 'Monthly Average Temperature'
                }
            }
        }
    });
});

