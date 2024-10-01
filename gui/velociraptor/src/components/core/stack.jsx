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
                    extra_columns={["Link"]}
                    columns={columns}
                    renderers={renderers}
                    params={{
                        type: "STACK",
                        stack_path: this.props.stack_path}}
                    transform={this.props.transform}
                    setTransform={this.props.setTransform}
                  />
                </Modal.Body>
              </Modal>
            </>
        );
    }
}
