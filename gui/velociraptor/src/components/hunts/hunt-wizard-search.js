import React from 'react';
import PropTypes from 'prop-types';

import Navbar from 'react-bootstrap/Navbar';
import Button from 'react-bootstrap/Button';

export default class HuntWizardStep1 extends React.Component {
    static propTypes = {

    };

    render() {
        return (
            <>
              <div className="WizardPage">
                <Navbar >
                  <Button variant="default"
                          onClick={this.props.prevStep}>
                    Prev
                  </Button>
                  <Button variant="default"
                          onClick={this.props.nextStep}>
                    Next
                  </Button>
                </Navbar>
              </div>

            </>
        );
    }
};
