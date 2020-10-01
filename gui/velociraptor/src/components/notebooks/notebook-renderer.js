import React from 'react';
import PropTypes from 'prop-types';

import NotebookCellRenderer from './notebook-cell-renderer.js';

import _ from 'lodash';

export default class NotebookRenderer extends React.Component {
    static propTypes = {
        notebook: PropTypes.object,
    };

    state = {
        selected_cell_id: "",
    }

    setSelectedCellId = (cell_id) => {
        this.setState({selected_cell_id: cell_id});
    }

    render() {
        return (
            <>
              { _.map(this.props.notebook.cell_metadata, (cell_md, idx) => {
                  return <NotebookCellRenderer
                           selected_cell_id={this.state.selected_cell_id}
                           setSelectedCellId={this.setSelectedCellId}
                           notebook_id={this.props.notebook.notebook_id}
                           cell_metadata={cell_md} key={idx} />;
              })}
            </>
        );
    }
};
