import React, { Component } from 'react';
import PropTypes from 'prop-types';

import Modal from 'react-bootstrap/Modal';
import T from '../i8n/i8n.jsx';
import VeloPagedTable from './paged-table.jsx';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';


export default class StackDialog extends Component {
    static propTypes = {
        name: PropTypes.string,
        onClose: PropTypes.func.isRequired,
        navigateToRow: PropTypes.func.isRequired,
        stack_path: PropTypes.array,

        // Save the transform in our parent so we can resume easily.
        transform: PropTypes.object,
        setTransform: PropTypes.func,
        setPageSize: PropTypes.func,

        // Start the stack table at this initial row.
        start_row: PropTypes.number,
        page_size: PropTypes.number,
    }

    navigate = row=>{
        this.props.navigateToRow(row);
        this.props.onClose();
    }

    render() {
        let renderers = {
            Link: (cell, row, rowIndex) => {
                return <Button variant="outline-dark"
                         onClick={()=>this.navigate(row.idx)}
                       >
                         <FontAwesomeIcon icon="bullseye"/>
                       </Button>;
            }
        };
        let name = this.props.name || T("Value");
        let columns = {
            value: {text: name},
            idx: {text: T("Row Index")},
            c:{text: T("Count")},
        };
        return (
            <>
              <Modal show={true}
                     className="full-height"
                     enforceFocus={false}
                     scrollable={true}
                     dialogClassName="modal-90w"
                     onHide={(e) => this.props.onClose()}>
                <Modal.Header closeButton>
                  <Modal.Title>{T("Stacking of column ") + name}</Modal.Title>
                </Modal.Header>
                <Modal.Body>
                  <VeloPagedTable
                    name={(this.props.name || "") + "Stack" }
                    extra_columns={["Link"]}
                    columns={columns}
                    renderers={renderers}
                    params={{
                        type: "STACK",
                        stack_path: this.props.stack_path}}
                    transform={this.props.transform}
                    setTransform={this.props.setTransform}

                    // Allow the caller to keep strack of the stack
                    // table's paging.
                    setPageState={this.props.setPageState}

                    // Start the table with these parameters.
                    initial_page_size={this.props.page_size}
                    initial_start_row={this.props.start_row}
                  />
                </Modal.Body>
              </Modal>
            </>
        );
    }
}
