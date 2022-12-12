import React, { Component } from 'react';
import PropTypes from 'prop-types';

import Button from 'react-bootstrap/Button';
import Modal from 'react-bootstrap/Modal';

export default class VeloNotImplemented extends Component {
    static propTypes = {
        show: PropTypes.bool,
        resolve: PropTypes.func,
    }

    render() {
        return (
            <Modal show={this.props.show} onHide={() => this.props.resolve()}>
              <Modal.Dialog>
                <Modal.Header closeButton>
                  <Modal.Title>Not Implemented</Modal.Title>
                </Modal.Header>

                <Modal.Body>
                  <p>Sorry this feature is not implemented yet!</p>
                </Modal.Body>

                <Modal.Footer>
                  <Button variant="secondary">Close</Button>
                </Modal.Footer>
              </Modal.Dialog>
            </Modal>
        );
    };
}
