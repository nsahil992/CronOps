document.addEventListener('DOMContentLoaded', function() {
    // DOM elements
    const minutesInput = document.getElementById('minutes');
    const hoursInput = document.getElementById('hours');
    const dayOfMonthInput = document.getElementById('dayOfMonth');
    const monthInput = document.getElementById('month');
    const dayOfWeekInput = document.getElementById('dayOfWeek');
    const fullCronInput = document.getElementById('fullCron');
    const convertBtn = document.getElementById('convertBtn');
    const saveBtn = document.getElementById('saveBtn');
    const humanReadableOutput = document.getElementById('humanReadable');
    const executionsList = document.getElementById('executionsList');
    const savedExpressionsList = document.getElementById('savedExpressionsList');
    const presetButtons = document.querySelectorAll('.presets button');
    const saveModal = document.getElementById('saveModal');
    const closeModal = document.querySelector('.close');
    const confirmSaveBtn = document.getElementById('confirmSave');
    const expressionNameInput = document.getElementById('expressionName');
    const expressionDescriptionInput = document.getElementById('expressionDescription');

    // Initialize
    loadSavedExpressions();
    updateFullCron();
    convertExpression();

    // Event listeners for individual cron fields
    [minutesInput, hoursInput, dayOfMonthInput, monthInput, dayOfWeekInput].forEach(input => {
        input.addEventListener('input', function() {
            updateFullCron();
        });
    });

    // Event listener for full cron expression
    fullCronInput.addEventListener('input', function() {
        updateIndividualInputs();
    });

    // Preset buttons
    presetButtons.forEach(button => {
        button.addEventListener('click', function() {
            fullCronInput.value = this.getAttribute('data-preset');
            updateIndividualInputs();
            convertExpression();
        });
    });

    // Convert button
    convertBtn.addEventListener('click', convertExpression);

    // Save button
    saveBtn.addEventListener('click', function() {
        openSaveModal();
    });

    // Close modal
    closeModal.addEventListener('click', function() {
        saveModal.style.display = 'none';
    });

    // Click outside modal to close
    window.addEventListener('click', function(event) {
        if (event.target === saveModal) {
            saveModal.style.display = 'none';
        }
    });

    // Confirm save
    confirmSaveBtn.addEventListener('click', saveExpression);

    // Update full cron expression from individual fields
    function updateFullCron() {
        const minutes = minutesInput.value.trim() || '*';
        const hours = hoursInput.value.trim() || '*';
        const dayOfMonth = dayOfMonthInput.value.trim() || '*';
        const month = monthInput.value.trim() || '*';
        const dayOfWeek = dayOfWeekInput.value.trim() || '*';
        
        fullCronInput.value = `${minutes} ${hours} ${dayOfMonth} ${month} ${dayOfWeek}`;
    }

    // Update individual fields from full cron expression
    function updateIndividualInputs() {
        const parts = fullCronInput.value.trim().split(/\s+/);
        
        if (parts.length === 5) {
            minutesInput.value = parts[0];
            hoursInput.value = parts[1];
            dayOfMonthInput.value = parts[2];
            monthInput.value = parts[3];
            dayOfWeekInput.value = parts[4];
        }
    }

    // Convert cron expression to human readable format and show next execution times
    function convertExpression() {
        const cronExpression = fullCronInput.value.trim();
        
        fetch('/api/convert', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({ expression: cronExpression }),
        })
        .then(response => {
            if (!response.ok) {
                throw new Error('Network response was not ok');
            }
            return response.json();
        })
        .then(data => {
            humanReadableOutput.innerHTML = `<p>${data.description}</p>`;
            
            // Display next execution times
            executionsList.innerHTML = '';
            data.nextExecutions.forEach(execution => {
                const li = document.createElement('li');
                li.textContent = execution;
                executionsList.appendChild(li);
            });
        })
        .catch(error => {
            humanReadableOutput.innerHTML = `<p class="error">Error: ${error.message}</p>`;
            console.error('Error:', error);
        });
    }

    // Open save modal
    function openSaveModal() {
        expressionNameInput.value = '';
        expressionDescriptionInput.value = '';
        saveModal.style.display = 'block';
    }

    // Save expression
    function saveExpression() {
        const name = expressionNameInput.value.trim();
        const expression = fullCronInput.value.trim();
        const description = expressionDescriptionInput.value.trim();
        
        if (!name) {
            alert('Please enter a name for this expression');
            return;
        }
        
        fetch('/api/expressions', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({
                name: name,
                expression: expression,
                description: description
            }),
        })
        .then(response => {
            if (!response.ok) {
                throw new Error('Network response was not ok');
            }
            return response.json();
        })
        .then(data => {
            saveModal.style.display = 'none';
            loadSavedExpressions();
        })
        .catch(error => {
            alert(`Error saving expression: ${error.message}`);
            console.error('Error:', error);
        });
    }

    // Load saved expressions
    function loadSavedExpressions() {
        fetch('/api/expressions')
        .then(response => {
            if (!response.ok) {
                throw new Error('Network response was not ok');
            }
            return response.json();
        })
        .then(data => {
            savedExpressionsList.innerHTML = '';
            
            data.forEach(item => {
                const row = document.createElement('tr');
                
                row.innerHTML = `
                    <td>${item.name}</td>
                    <td>${item.expression}</td>
                    <td>${item.description}</td>
                    <td>
                        <button class="action-btn load-btn" data-id="${item.id}" data-expression="${item.expression}">Load</button>
                        <button class="action-btn edit-btn" data-id="${item.id}">Edit</button>
                        <button class="action-btn delete-btn" data-id="${item.id}">Delete</button>
                    </td>
                `;
                
                savedExpressionsList.appendChild(row);
            });
            
            // Add event listeners to buttons
            document.querySelectorAll('.load-btn').forEach(button => {
                button.addEventListener('click', function() {
                    fullCronInput.value = this.getAttribute('data-expression');
                    updateIndividualInputs();
                    convertExpression();
                });
            });
            
            document.querySelectorAll('.delete-btn').forEach(button => {
                button.addEventListener('click', function() {
                    deleteExpression(this.getAttribute('data-id'));
                });
            });
            
            document.querySelectorAll('.edit-btn').forEach(button => {
                button.addEventListener('click', function() {
                    editExpression(this.getAttribute('data-id'));
                });
            });
        })
        .catch(error => {
            console.error('Error:', error);
            savedExpressionsList.innerHTML = `<tr><td colspan="4">Error loading saved expressions: ${error.message}</td></tr>`;
        });
    }

    // Delete expression
    function deleteExpression(id) {
        if (confirm('Are you sure you want to delete this expression?')) {
            fetch(`/api/expressions/${id}`, {
                method: 'DELETE',
            })
            .then(response => {
                if (!response.ok) {
                    throw new Error('Network response was not ok');
                }
                return response.json();
            })
            .then(data => {
                loadSavedExpressions();
            })
            .catch(error => {
                alert(`Error deleting expression: ${error.message}`);
                console.error('Error:', error);
            });
        }
    }

    // Edit expression
    function editExpression(id) {
        fetch(`/api/expressions/${id}`)
        .then(response => {
            if (!response.ok) {
                throw new Error('Network response was not ok');
            }
            return response.json();
        })
        .then(data => {
            expressionNameInput.value = data.name;
            expressionDescriptionInput.value = data.description;
            
            // Custom attribute to store the ID for update
            confirmSaveBtn.setAttribute('data-edit-id', id);
            saveModal.style.display = 'block';
            
            // Override the confirm button's click event
            confirmSaveBtn.removeEventListener('click', saveExpression);
            confirmSaveBtn.addEventListener('click', updateExpression);
        })
        .catch(error => {
            alert(`Error loading expression details: ${error.message}`);
            console.error('Error:', error);
        });
    }

    // Update expression
    function updateExpression() {
        const id = confirmSaveBtn.getAttribute('data-edit-id');
        const name = expressionNameInput.value.trim();
        const expression = fullCronInput.value.trim();
        const description = expressionDescriptionInput.value.trim();
        
        if (!name) {
            alert('Please enter a name for this expression');
            return;
        }
        
        fetch(`/api/expressions/${id}`, {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({
                name: name,
                expression: expression,
                description: description
            }),
        })
        .then(response => {
            if (!response.ok) {
                throw new Error('Network response was not ok');
            }
            return response.json();
        })
        .then(data => {
            saveModal.style.display = 'none';
            loadSavedExpressions();
            
            // Reset confirm button event
            confirmSaveBtn.removeEventListener('click', updateExpression);
            confirmSaveBtn.addEventListener('click', saveExpression);
        })
        .catch(error => {
            alert(`Error updating expression: ${error.message}`);
            console.error('Error:', error);
        });
    }
});
