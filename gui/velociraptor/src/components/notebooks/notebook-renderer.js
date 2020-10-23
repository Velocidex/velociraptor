import React from 'react';
import PropTypes from 'prop-types';

import NotebookCellRenderer from './notebook-cell-renderer.js';

import _ from 'lodash';

import api from '../core/api-service.js';

export default class NotebookRenderer extends React.Component {
    static propTypes = {
        notebook: PropTypes.object,
        fetchNotebooks: PropTypes.func,
    };

    state = {
        selected_cell_id: "",
    }

    setSelectedCellId = (cell_id) => {
        this.setState({selected_cell_id: cell_id});
    }


    upCell = (cell_id) => {
        let cell_metadata = this.props.notebook.cell_metadata;
        let changed = false;

        let new_cells = [];
        for (var i=0; i<cell_metadata.length; i++) {
            if (cell_metadata[i].cell_id === cell_id && new_cells.length > 0) {
                let last_cell = new_cells.pop();
                new_cells.push(cell_metadata[i]);
                new_cells.push(last_cell);
                changed = true;
            } else {
                new_cells.push(cell_metadata[i]);
            }
        }

        if (changed) {
            this.props.notebook.cell_metadata = new_cells;
            api.post('v1/UpdateNotebook', this.props.notebook).then((response) => {
                    this.props.fetchNotebooks();

                }, (response) => {
                    console.log("Error " + response.data);
                });
        }
    };

    deleteCell = (cell_id) => {
        var changed = false;
        var cell_metadata = this.props.notebook.cell_metadata;

        // Dont allow us to remove all cells.
        if (cell_metadata.length <= 1) {
            return;
        }

        var new_cells = [];
        for (var i=0; i<cell_metadata.length; i++) {
            if (cell_metadata[i].cell_id === cell_id) {
                changed = true;
            } else {
                new_cells.push(cell_metadata[i]);
            }
        }

        if (changed) {
            this.props.notebook.cell_metadata = new_cells;

            api.post('v1/UpdateNotebook', this.props.notebook).then((response) => {
                    this.props.fetchNotebooks();

                }, function failure(response) {
                    console.log("Error " + response.data);
                });
        }
    };

    downCell = (cell_id) => {
        var changed = false;
        var cell_metadata = this.props.notebook.cell_metadata;

        var new_cells = [];
        for (var i=0; i<cell_metadata.length; i++) {
            if (cell_metadata[i].cell_id === cell_id && cell_metadata.length > i) {
                var next_cell = cell_metadata[i+1];
                if (!_.isEmpty(next_cell)) {
                    new_cells.push(next_cell);
                    new_cells.push(cell_metadata[i]);
                    i += 1;
                    changed = true;
                }
            } else {
                new_cells.push(cell_metadata[i]);
            }
        }

        if (changed) {
            this.props.notebook.cell_metadata = new_cells;

            api.post('v1/UpdateNotebook', this.props.notebook).then((response) => {
                    this.props.fetchNotebooks();

                }, function failure(response) {
                    console.log("Error " + response.data);
                });
        }
    };

    addCell = (cell_id, cell_type, content) => {
        let request = {};
        switch(cell_type) {
        case "VQL":
        case "Markdown":
        case "Artifact":
            request = {
                notebook_id: this.props.notebook.notebook_id,
                type: cell_type,
                cell_id: cell_id,
                input: content,
            }; break;
        default:
            return;
        }

        api.post('v1/NewNotebookCell', request).then((response) => {
            this.props.fetchNotebooks();
            this.setState({selected_cell_id: response.data.latest_cell_id});
        });
    }

    render() {
        if (!this.props.notebook || _.isEmpty(this.props.notebook.cell_metadata)) {
            return <h5 className="no-content">Select a notebook from the list above.</h5>;
        }

        return (
            <>
              { _.map(this.props.notebook.cell_metadata, (cell_md, idx) => {
                  return <NotebookCellRenderer
                           selected_cell_id={this.state.selected_cell_id}
                           setSelectedCellId={this.setSelectedCellId}
                           notebook_id={this.props.notebook.notebook_id}
                           cell_metadata={cell_md} key={idx}
                           upCell={this.upCell}
                           downCell={this.downCell}
                           deleteCell={this.deleteCell}
                           addCell={this.addCell}
                      />;
              })}
            </>
        );
    }
};
